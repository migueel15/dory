package app

import (
	"context"
	"errors"
	"strings"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/runtime"
	"github.com/migueel15/dory/internal/workspace"
)

// Spawn registers the delegation on the parent's node before starting the child.
func (s *Threads) Spawn(ctx context.Context, parentID string, in CreateThread) (data.Thread, runtime.Delegation, error) {
	var delegation runtime.Delegation
	parent, err := s.Store.Thread(ctx, parentID)
	if err != nil {
		return data.Thread{}, delegation, err
	}
	prompt := in.Text
	if strings.TrimSpace(prompt) == "" {
		return data.Thread{}, delegation, data.ErrInvalid.Msgf("the subagent needs a prompt")
	}
	if err := s.Runtime.CanSpawn(ctx, parent); err != nil {
		return data.Thread{}, delegation, err
	}
	if in.Node == "" {
		in.Node = parent.NodeID
	}
	if in.ProjectID == nil {
		in.ProjectID = parent.ProjectID
	}
	if in.TaskID == nil {
		in.TaskID = parent.TaskID
	}
	if in.Title == "" {
		in.Title = shortTitle(prompt)
	}
	in.ParentID, in.Inherit, in.Text = &parent.ID, &parent, ""
	child, err := s.Create(ctx, in)
	if err != nil {
		return child, delegation, err
	}
	if child.Title == "" {
		child.Title = in.Title
	}
	delegation, err = s.Runtime.RecordDelegation(ctx, parent.ID, child.ID, child.NodeID, child.Title)
	if err != nil {
		return child, delegation, createdError(child, err)
	}
	return child, delegation, s.sendInitial(ctx, child, prompt)
}

// Adopt registers a conversation already running in an external harness.
func (s *Threads) Adopt(ctx context.Context, e runtime.External, projectID *string) (data.Thread, error) {
	t := data.Thread{ID: data.NewThreadID(), NodeID: s.Node, ProjectID: projectID, Provider: e.Provider, Title: e.Title, Branch: e.Branch}
	return s.Runtime.RegisterExternal(ctx, t, e)
}

type ArchiveOptions struct {
	KeepWorktree bool
}

// Archive stops the thread on its owner and removes its clean worktree when
// configured. A dirty or shared worktree is left available for later use.
func (s *Threads) Archive(ctx context.Context, id string, opts ArchiveOptions) error {
	t, err := s.Store.Thread(ctx, id)
	if err != nil {
		return err
	}
	if t.NodeID != s.Node {
		if s.Peers == nil {
			return apperr.New(apperr.NodeOffline, "the node that runs this thread is not connected").With("thread", true)
		}
		return s.Peers.ArchiveThread(ctx, t.NodeID, id, opts.KeepWorktree)
	}
	_ = s.Runtime.Interrupt(ctx, id)
	s.Runtime.CloseThread(id)
	s.Runtime.DropSnapshots(ctx, id)
	if !opts.KeepWorktree && s.Settings.ThreadSettings().RemoveWorktreeOnArchive {
		err := s.Runtime.Remove(ctx, id, false)
		if err != nil && !errors.Is(err, workspace.ErrDirtyWorktree) && !errors.Is(err, runtime.ErrNotFound) {
			return err
		}
	}
	return s.Store.ArchiveThread(ctx, id)
}
