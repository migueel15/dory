package api

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/runtime"
	"github.com/migueel15/dory/internal/workspace"
)

const maintenanceEvery = 2 * time.Minute

// StartWorktreeMaintenance pulls branches and removes worktrees of merged
// work according to the node settings.
func (s *Server) StartWorktreeMaintenance(ctx context.Context) {
	go func() {
		t := time.NewTicker(maintenanceEvery)
		defer t.Stop()
		for {
			s.AutoPull(ctx)
			s.RemoveMergedWorktrees(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

// AutoPull fast-forwards the branch of every active thread of this node that
// runs in a checkout without uncommitted changes, when the node is set to.
func (s *Server) AutoPull(ctx context.Context) {
	if !s.Settings.Get().SourceControl.AutoPull {
		return
	}
	threads, err := s.Store.Threads().ListAll(ctx, data.Filter{})
	if err != nil {
		return
	}
	seen := map[string]bool{}
	for _, t := range threads {
		if t.NodeID != s.Node.ID || t.Branch == "" || s.Runtime.Running(t.ID) {
			continue
		}
		if dir, err := s.Runtime.Dir(ctx, t.ID); err == nil && dir != "" && !seen[dir] {
			seen[dir] = true
			if err := workspace.Pull(ctx, dir, t.Branch); err != nil {
				log.Printf("api: auto-pull %s: %v", t.Branch, err)
			}
		}
	}
}

// RemoveMergedWorktrees deletes the clean worktrees of the threads of a task
// whose pull request was merged, when the node is set to. A worktree with
// changes, or one another active thread uses, is kept.
func (s *Server) RemoveMergedWorktrees(ctx context.Context) {
	if !s.Settings.Get().Worktrees.RemoveOnMerge {
		return
	}
	threads, err := s.Store.Threads().ListAll(ctx, data.Filter{IncludeArchived: true})
	if err != nil {
		return
	}
	byTask := map[string][]data.Thread{}
	var tasks []data.Task
	for _, th := range threads {
		if th.TaskID == nil || th.NodeID != s.Node.ID {
			continue
		}
		if _, ok := byTask[*th.TaskID]; !ok {
			if t, err := s.Store.Tasks().Get(ctx, *th.TaskID); err == nil {
				tasks = append(tasks, t)
			}
		}
		byTask[*th.TaskID] = append(byTask[*th.TaskID], th)
	}
	cards, err := s.Cards(ctx, tasks)
	if err != nil {
		return
	}
	for _, c := range cards {
		if c.PRState != "merged" {
			continue
		}
		for _, th := range byTask[c.ID] {
			if err := s.Runtime.Remove(ctx, th.ID, false); err != nil && !errors.Is(err, workspace.ErrDirtyWorktree) && !errors.Is(err, runtime.ErrNotFound) {
				log.Printf("api: remove merged worktree of %s: %v", th.ID, err)
			}
		}
	}
}
