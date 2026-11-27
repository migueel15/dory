package app

import (
	"context"
	"fmt"

	"github.com/migueel15/dory/internal/data"
)

// MigrateLegacyTasks migrates legacy tasks that still have issue_number,
// pr_number or branch into the multi-source model (source=github with
// deterministic UUIDv5, and thread_pulls for linked PRs). It is deterministic
// and idempotent so concurrent nodes converge to the same state.
func (a *Tasks) MigrateLegacyTasks(ctx context.Context) error {
	tasks, err := a.Store.Tasks().ListAll(ctx, data.Filter{})
	if err != nil {
		return err
	}
	var legacy []data.Task
	for _, t := range tasks {
		if t.IssueNumber != nil || t.PRNumber != nil || t.Branch != "" {
			legacy = append(legacy, t)
		}
	}
	if len(legacy) == 0 {
		return nil
	}
	allThreads, err := a.Store.Threads().ListAll(ctx, data.Filter{})
	if err != nil {
		return err
	}
	threadsByTask := map[string][]data.Thread{}
	for _, th := range allThreads {
		if th.TaskID != nil {
			threadsByTask[*th.TaskID] = append(threadsByTask[*th.TaskID], th)
		}
	}
	allPlans, err := a.Store.Plans().ListAll(ctx, data.Filter{})
	if err != nil {
		return err
	}
	plansByTask := map[string][]data.Plan{}
	for _, p := range allPlans {
		plansByTask[p.TaskID] = append(plansByTask[p.TaskID], p)
	}
	allDeps, err := a.Store.Dependencies().ListAll(ctx, data.Filter{})
	if err != nil {
		return err
	}

	for _, t := range legacy {
		if t.IssueNumber != nil && t.RepositoryID != nil {
			newID := data.ExternalTaskID(*t.RepositoryID, *t.IssueNumber)
			ext, err := a.Store.EnsureExternalTask(ctx, t.ProjectID, *t.RepositoryID, *t.IssueNumber)
			if err != nil {
				return fmt.Errorf("migrate task %s: %w", t.ID, err)
			}
			patch := data.Fields{}
			if t.Stage == "ready" && ext.Stage == "" {
				patch["stage"] = "ready"
			}
			if t.Position != "" && ext.Position == "" {
				patch["position"] = t.Position
			}
			if len(patch) > 0 {
				if _, err := a.Store.Tasks().Patch(ctx, newID, patch); err != nil {
					return fmt.Errorf("migrate task %s patch: %w", t.ID, err)
				}
			}

			for _, th := range threadsByTask[t.ID] {
				if th.TaskID == nil || *th.TaskID != newID {
					if _, err := a.Store.Threads().Patch(ctx, th.ID, data.Fields{"task_id": newID}); err != nil {
						return fmt.Errorf("migrate thread %s: %w", th.ID, err)
					}
				}
				if t.PRNumber != nil {
					if _, err := a.Store.ThreadPulls().Link(ctx, th.ID, *t.RepositoryID, *t.PRNumber, th.Branch, "manual"); err != nil {
						return fmt.Errorf("migrate thread_pull %s: %w", th.ID, err)
					}
				}
			}
			for _, p := range plansByTask[t.ID] {
				if p.TaskID != newID {
					if _, err := a.Store.Plans().Patch(ctx, p.ID, data.Fields{"task_id": newID}); err != nil {
						return fmt.Errorf("migrate plan %s: %w", p.ID, err)
					}
				}
			}
			for _, dep := range allDeps {
				if dep.TaskID != t.ID && dep.DependsOnID != t.ID {
					continue
				}
				newTaskID := dep.TaskID
				if newTaskID == t.ID {
					newTaskID = newID
				}
				newDepID := dep.DependsOnID
				if newDepID == t.ID {
					newDepID = newID
				}
				oldKey := dep.TaskID + "/" + dep.DependsOnID
				if err := a.Store.Dependencies().Archive(ctx, oldKey); err != nil {
					return fmt.Errorf("migrate dep archive %s: %w", oldKey, err)
				}
				if newTaskID == newDepID {
					continue // discard self-dependency
				}
				if _, err := a.Store.Dependencies().Upsert(ctx, data.Dependency{
					TaskID:      newTaskID,
					DependsOnID: newDepID,
					CreatedAt:   dep.CreatedAt,
				}); err != nil {
					return fmt.Errorf("migrate dep upsert %s/%s: %w", newTaskID, newDepID, err)
				}
			}
			if t.ID != newID {
				if err := a.Store.Tasks().Archive(ctx, t.ID); err != nil {
					return fmt.Errorf("migrate archive old task %s: %w", t.ID, err)
				}
			} else {
				if _, err := a.Store.Tasks().Patch(ctx, t.ID, data.Fields{"issue_number": nil, "pr_number": nil, "branch": ""}); err != nil {
					return err
				}
			}
			continue
		}

		// Internal task with legacy pr_number or branch (or issue_number without repo).
		if t.PRNumber != nil && t.RepositoryID != nil {
			for _, th := range threadsByTask[t.ID] {
				if _, err := a.Store.ThreadPulls().Link(ctx, th.ID, *t.RepositoryID, *t.PRNumber, th.Branch, "manual"); err != nil {
					return fmt.Errorf("migrate internal thread_pull %s: %w", th.ID, err)
				}
			}
		}
		clear := data.Fields{}
		if t.IssueNumber != nil {
			clear["issue_number"] = nil
		}
		if t.PRNumber != nil {
			clear["pr_number"] = nil
		}
		if t.Branch != "" {
			clear["branch"] = ""
		}
		if len(clear) > 0 {
			if _, err := a.Store.Tasks().Patch(ctx, t.ID, clear); err != nil {
				return fmt.Errorf("clear legacy fields %s: %w", t.ID, err)
			}
		}
	}
	return nil
}
