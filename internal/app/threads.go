// Package app coordinates Dory's use cases across storage, runtime and nodes.
package app

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/mesh/peerapi"
	"github.com/migueel15/dory/internal/runtime"
	"github.com/migueel15/dory/internal/workspace"
)

type Threads struct {
	Store     ThreadStore
	Runtime   ThreadRuntime
	Workspace Workspace
	Settings  NodeSettings
	Peers     PeerClient
	Namer     BranchNamer
	Node      string
	DataDir   string
}

type RepositoryStore interface {
	ProjectRepository(context.Context, string) (string, error)
	RepositoryPath(context.Context, string) (string, error)
}

type ThreadStore interface {
	RepositoryStore
	Thread(context.Context, string) (data.Thread, error)
	ArchiveThread(context.Context, string) error
}

type ThreadRuntime interface {
	ResolveSettings(context.Context, data.Thread, *runtime.ThreadSettings) error
	Register(context.Context, data.Thread, string, string) (data.Thread, error)
	RegisterExternal(context.Context, data.Thread, runtime.External) (data.Thread, error)
	Send(context.Context, string, string, ...runtime.Attachment) error
	Dir(context.Context, string) (string, error)
	WorktreeUse(context.Context, string, string) (runtime.WorktreeUse, error)
	CanSpawn(context.Context, data.Thread) error
	RecordDelegation(context.Context, string, string, string, string) (runtime.Delegation, error)
	Interrupt(context.Context, string) error
	CloseThread(string)
	DropSnapshots(context.Context, string)
	Remove(context.Context, string, bool) error
}

type Workspace interface {
	Worktrees(context.Context, string) []workspace.Worktree
	DefaultBranch(context.Context, string) string
	BranchExists(context.Context, string, string) bool
	SwitchBranch(context.Context, string, string) error
	AddWorktree(context.Context, string, string, string, string) (bool, error)
	RemoveWorktree(context.Context, string, bool) error
	DeleteBranch(context.Context, string, string) error
	Pull(context.Context, string, string) error
}

type NodeSettings interface {
	ThreadSettings() ThreadSettings
}

type ThreadSettings struct {
	DefaultModel, FallbackModel                  ModelChoice
	DefaultWorkspace, WorktreeRoot, BranchNaming string
	AutoPull, RemoveWorktreeOnArchive            bool
}

type ModelChoice struct{ Provider, Model, Effort string }

type PeerClient interface {
	CreateThread(context.Context, string, peerapi.CreateThreadRequest) (peerapi.CreateThreadResponse, error)
	ArchiveThread(context.Context, string, string, bool) error
	SendMessage(context.Context, string, string, peerapi.SendMessageRequest) error
}

type CreateThread struct {
	Node string
	peerapi.CreateThreadRequest
}

// Create prepares the workspace before publishing the thread.
func (s *Threads) Create(ctx context.Context, in CreateThread) (data.Thread, error) {
	if in.Node == "" || in.Node == "local" {
		in.Node = s.Node
	}
	if in.Node != s.Node {
		return s.createRemote(ctx, in)
	}
	_, repo, err := RepositoryCheckout(ctx, s.Store, in.ProjectID, in.RepositoryID)
	if err != nil {
		return data.Thread{}, err
	}
	defaults := s.Settings.ThreadSettings()
	t := data.Thread{
		ID: data.NewThreadID(), NodeID: s.Node, ProjectID: in.ProjectID, TaskID: in.TaskID,
		ParentID: in.ParentID, OriginID: in.OriginID, Title: in.Title, Branch: in.Branch,
	}
	if t.Title == "" {
		t.Title = shortTitle(in.Text)
	}
	if t.TaskID != nil {
		at := time.Now().UTC().Format(time.RFC3339Nano)
		t.TaskLinkedAt = &at
	}
	if err := s.resolveSelection(ctx, &t, in, defaults); err != nil {
		return data.Thread{}, err
	}
	prepared, err := s.prepareWorkspace(ctx, &t, in, repo, defaults)
	if err != nil {
		return data.Thread{}, err
	}
	t, err = s.Runtime.Register(ctx, t, prepared.dir, prepared.base)
	if err != nil {
		return data.Thread{}, errors.Join(err, s.undoWorkspace(ctx, prepared))
	}
	if defaults.AutoPull && t.Branch != "" && prepared.dir != "" {
		if err := s.Workspace.Pull(ctx, prepared.dir, t.Branch); err != nil {
			log.Printf("threads: auto-pull %s: %v", t.Branch, err)
		}
	}
	return t, s.sendInitial(ctx, t, in.Text)
}

func (s *Threads) createRemote(ctx context.Context, in CreateThread) (data.Thread, error) {
	if s.Peers == nil {
		return data.Thread{}, apperr.New(apperr.NodeOffline, "the node is not connected")
	}
	out, err := s.Peers.CreateThread(ctx, in.Node, in.CreateThreadRequest)
	if out.Thread != nil {
		return *out.Thread, err
	}
	if e := apperr.As(err); e != nil && e.Code == apperr.ThreadCreatedSendFailed {
		out.ThreadID, _ = e.Params["thread_id"].(string)
	}
	return data.Thread{ID: out.ThreadID, NodeID: in.Node}, err
}

func (s *Threads) resolveSelection(ctx context.Context, t *data.Thread, in CreateThread, defaults ThreadSettings) error {
	choice := defaults.DefaultModel
	if in.Inherit != nil && in.Inherit.Provider != "" {
		choice = ModelChoice{in.Inherit.Provider, in.Inherit.Model, in.Inherit.Effort}
		t.RuntimeMode = in.Inherit.RuntimeMode
	}
	if in.RuntimeMode != "" {
		t.RuntimeMode = in.RuntimeMode
	}
	if provider := strings.ToLower(strings.TrimSpace(in.Provider)); provider != "" && provider != choice.Provider {
		if in.Inherit != nil && in.Inherit.Provider != "" && in.Model == "" {
			return data.ErrInvalid.Msgf("choose a model for provider %s; use list_models", provider)
		}
		choice = ModelChoice{Provider: provider}
	}
	if in.Model != "" {
		if in.Model != choice.Model {
			choice.Effort = ""
		}
		choice.Model = in.Model
	}
	if in.Effort != "" {
		choice.Effort = in.Effort
	}
	err := s.applySelection(ctx, t, choice)
	fallback := defaults.FallbackModel
	if e := apperr.As(err); e != nil && e.Code == apperr.ProviderUnavailable && fallback.Provider != "" && fallback.Provider != choice.Provider {
		return s.applySelection(ctx, t, fallback)
	}
	return err
}

func (s *Threads) applySelection(ctx context.Context, t *data.Thread, choice ModelChoice) error {
	settings := runtime.ThreadSettings{Provider: &choice.Provider}
	if choice.Model != "" {
		settings.Model = &choice.Model
	}
	if choice.Effort != "" {
		settings.Effort = &choice.Effort
	}
	if err := s.Runtime.ResolveSettings(ctx, *t, &settings); err != nil {
		return err
	}
	t.Provider, t.Model, t.Effort = *settings.Provider, choice.Model, choice.Effort
	if settings.Model != nil {
		t.Model = *settings.Model
	}
	if settings.Effort != nil {
		t.Effort = *settings.Effort
	}
	return nil
}

func (s *Threads) sendInitial(ctx context.Context, t data.Thread, text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var err error
	if t.NodeID == s.Node {
		err = s.Runtime.Send(ctx, t.ID, text)
	} else {
		err = s.Peers.SendMessage(ctx, t.NodeID, t.ID, peerapi.SendMessageRequest{Text: text})
	}
	return createdError(t, err)
}

func createdError(t data.Thread, err error) error {
	if err == nil {
		return nil
	}
	return apperr.Wrap(apperr.ThreadCreatedSendFailed, err, "the thread was created but its first message could not be sent").With("thread_id", t.ID)
}

func shortTitle(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	if r := []rune(line); len(r) > 60 {
		line = string(r[:59]) + "…"
	}
	if line == "" {
		return "Nuevo hilo"
	}
	return line
}
