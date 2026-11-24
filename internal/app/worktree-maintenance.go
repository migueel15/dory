package app

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
func (a *Tasks) StartWorktreeMaintenance(ctx context.Context) {
	go func() {
		t := time.NewTicker(maintenanceEvery)
		defer t.Stop()
		for {
			a.AutoPull(ctx)
			a.RemoveMergedWorktrees(ctx)
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
func (a *Tasks) AutoPull(ctx context.Context) {
	th := a.Threads
	if !th.Settings.ThreadSettings().AutoPull {
		return
	}
	threads, err := a.Store.Threads().ListAll(ctx, data.Filter{})
	if err != nil {
		return
	}
	seen := map[string]bool{}
	for _, t := range threads {
		if t.NodeID != a.Node || t.Branch == "" || th.Runtime.Running(t.ID) {
			continue
		}
		if dir, err := th.Runtime.Dir(ctx, t.ID); err == nil && dir != "" && !seen[dir] {
			seen[dir] = true
			if err := th.Workspace.Pull(ctx, dir, t.Branch); err != nil {
				log.Printf("app: auto-pull %s: %v", t.Branch, err)
			}
		}
	}
}

// RemoveMergedWorktrees deletes the clean worktrees of the threads of a task
// whose pull request was merged, when the node is set to. A worktree with
// changes, or one another active thread uses, is kept.
func (a *Tasks) RemoveMergedWorktrees(ctx context.Context) {
	if !a.Threads.Settings.ThreadSettings().RemoveWorktreeOnMerge {
		return
	}
	threads, err := a.Store.Threads().ListAll(ctx, data.Filter{IncludeArchived: true})
	if err != nil {
		return
	}
	byTask := map[string][]data.Thread{}
	var tasks []data.Task
	for _, th := range threads {
		if th.TaskID == nil || th.NodeID != a.Node {
			continue
		}
		if _, ok := byTask[*th.TaskID]; !ok {
			if t, err := a.Store.Tasks().Get(ctx, *th.TaskID); err == nil {
				tasks = append(tasks, t)
			}
		}
		byTask[*th.TaskID] = append(byTask[*th.TaskID], th)
	}
	cards, err := a.Cards(ctx, tasks)
	if err != nil {
		return
	}
	for _, c := range cards {
		if c.PRState != "merged" {
			continue
		}
		for _, th := range byTask[c.ID] {
			if err := a.Threads.Runtime.Remove(ctx, th.ID, false); err != nil && !errors.Is(err, workspace.ErrDirtyWorktree) && !errors.Is(err, runtime.ErrNotFound) {
				log.Printf("app: remove merged worktree of %s: %v", th.ID, err)
			}
		}
	}
}
