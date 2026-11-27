package app

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/github"
)

var taskStages = map[string]bool{"inbox": true, "ready": true, "done": true, "cancelled": true}

// Create validates the initial stage and applies its side effects.
func (a *Tasks) Create(ctx context.Context, t data.Task) (data.Task, error) {
	if t.Stage != "" && !taskStages[t.Stage] {
		return t, invalidTaskStage()
	}
	if t.Stage == "done" {
		if err := a.CheckDone(ctx, t); err != nil {
			return t, err
		}
	}
	out, err := a.Store.Tasks().Create(ctx, t)
	if err == nil && (out.Stage == "done" || out.Stage == "cancelled") {
		a.FinishTask(ctx, out.ID)
	}
	return out, err
}

// Patch checks stage transitions and applies them against the version
// used for the check, so a concurrent edit cannot invalidate the decision.
func (a *Tasks) Patch(ctx context.Context, id string, expected *string, fields data.Fields) (data.Task, error) {
	tasks := a.Store.Tasks()
	// The first write on an external item adopts it; the version the client
	// sent was the item's, not the new task's.
	cur, err := tasks.Get(ctx, id)
	if errors.Is(err, data.ErrNotFound) {
		if cur, err = a.Adopt(ctx, id); err != nil {
			return cur, err
		}
		expected = nil
	} else if err != nil {
		return cur, err
	}
	if v, ok := fields["issue_number"]; ok {
		delete(fields, "issue_number")
		if cur, err = a.attachIssueField(ctx, cur, fields, v); err != nil {
			return cur, err
		}
		expected = nil
		if len(fields) == 0 {
			return cur, nil
		}
	}
	external := cur.Source != "" && cur.Source != "internal"
	if external {
		// Title and description belong to the external item; the row keeps a
		// copy for nodes without its cache.
		var in TaskUpdateInput
		if v, ok := fields["title"].(string); ok {
			in.Title = &v
		}
		if v, ok := fields["description"].(string); ok {
			in.Description = &v
		}
		if in.Title != nil || in.Description != nil {
			p, _, err := a.ProviderFor(ctx, id)
			if err != nil {
				return cur, err
			}
			if _, err := p.Update(ctx, id, in); err != nil {
				return cur, err
			}
		}
	}
	stage, hasStage := fields["stage"]
	if hasStage && external {
		// Done and cancelled close the external item; its column follows it.
		next, _ := stage.(string)
		if next == "inbox" || next == "ready" {
			// Back to work: a closed item is reopened in its provider.
			if _, u, err := a.ProviderFor(ctx, id); err == nil && u.State == "closed" {
				if _, err := a.SetState(ctx, id, TaskStateInput{State: "open"}); err != nil {
					return cur, err
				}
			}
		}
		if next == "done" || next == "cancelled" {
			if _, err := a.SetState(ctx, id, TaskStateInput{State: next}); err != nil {
				return cur, err
			}
			delete(fields, "stage")
			if len(fields) == 0 {
				return tasks.Get(ctx, id)
			}
			hasStage = false
		}
	}
	if !hasStage {
		if expected != nil {
			return tasks.PatchIf(ctx, id, *expected, fields)
		}
		return tasks.Patch(ctx, id, fields)
	}
	next, ok := stage.(string)
	if !ok || !taskStages[next] {
		return data.Task{}, invalidTaskStage()
	}
	current, err := tasks.Get(ctx, id)
	if err != nil {
		return current, err
	}
	if expected != nil && *expected != current.UpdatedAt {
		return current, data.ErrRevisionConflict
	}
	changed := current.Stage != next
	if changed && next == "done" {
		// Check the pull request and repository as they will be after the patch.
		candidate := current
		for field, dst := range map[string]any{"repository_id": &candidate.RepositoryID, "pr_number": &candidate.PRNumber, "branch": &candidate.Branch} {
			if value, ok := fields[field]; ok {
				raw, err := json.Marshal(value)
				if err != nil || json.Unmarshal(raw, dst) != nil {
					return current, data.ErrInvalid.Msgf("invalid %s", field)
				}
			}
		}
		if err := a.CheckDone(ctx, candidate); err != nil {
			return current, err
		}
	}
	out, err := tasks.PatchIf(ctx, id, current.UpdatedAt, fields)
	if err == nil && changed && (out.Stage == "done" || out.Stage == "cancelled") {
		a.FinishTask(ctx, out.ID)
	}
	if err == nil && changed && out.Stage == "inbox" {
		// Back to the inbox, an accepted plan waits to be accepted again.
		err = a.unacceptPlans(ctx, out.ID)
	}
	return out, err
}

func (a *Tasks) unacceptPlans(ctx context.Context, taskID string) error {
	plans, err := a.Store.Plans().ListAll(ctx, data.Filter{Fields: map[string]any{"task_id": taskID}})
	if err != nil {
		return err
	}
	for _, p := range plans {
		if p.AcceptedAt != nil {
			if _, err := a.Store.Plans().Patch(ctx, p.ID, data.Fields{"accepted_at": nil}); err != nil {
				return err
			}
		}
	}
	return nil
}

// attachIssueField turns an issue number written on a task into the key of
// that issue of the task's repository.
func (a *Tasks) attachIssueField(ctx context.Context, t data.Task, fields data.Fields, v any) (data.Task, error) {
	raw, _ := json.Marshal(v)
	var number *int64
	if err := json.Unmarshal(raw, &number); err != nil {
		return t, data.ErrInvalid.Msgf("invalid issue_number")
	}
	if number == nil || *number <= 0 {
		return t, nil
	}
	repoID := ""
	if t.RepositoryID != nil {
		repoID = *t.RepositoryID
	}
	if r, ok := fields["repository_id"].(string); ok && r != "" {
		repoID = r
	}
	if repoID == "" {
		return t, data.ErrInvalid.Msgf("the task needs a repository to link an issue")
	}
	return a.AttachIssue(ctx, t.ID, repoID, *number)
}

// SetState opens, completes or cancels a task through its provider. An
// external item is adopted first.
func (a *Tasks) SetState(ctx context.Context, id string, in TaskStateInput) (UnifiedTask, error) {
	if _, err := a.Adopt(ctx, id); err != nil {
		return UnifiedTask{}, err
	}
	p, _, err := a.ProviderFor(ctx, id)
	if err != nil {
		return UnifiedTask{}, err
	}
	return p.SetState(ctx, id, in)
}

// Comment writes on the task's external item. The item is adopted first.
func (a *Tasks) Comment(ctx context.Context, id, body string) (github.IssueComment, error) {
	if _, err := a.Adopt(ctx, id); err != nil {
		return github.IssueComment{}, err
	}
	p, _, err := a.ProviderFor(ctx, id)
	if err != nil {
		return github.IssueComment{}, err
	}
	return p.Comment(ctx, id, body)
}

func invalidTaskStage() error {
	return data.ErrInvalid.Msgf("stage must be inbox, ready, done or cancelled")
}

// LinkThread links a thread to a task, adopting an external item first.
func (a *Tasks) LinkThread(ctx context.Context, threadID, taskID string, replace bool) (data.Thread, error) {
	if _, err := a.Adopt(ctx, taskID); err != nil {
		return data.Thread{}, err
	}
	return a.Store.LinkThread(ctx, threadID, taskID, replace)
}

// AddDependency makes taskID wait for dependsOnID, adopting external items first.
func (a *Tasks) AddDependency(ctx context.Context, taskID, dependsOnID string) (data.Dependency, error) {
	for _, id := range []string{taskID, dependsOnID} {
		if _, err := a.Adopt(ctx, id); err != nil {
			return data.Dependency{}, err
		}
	}
	return a.Store.AddDependency(ctx, taskID, dependsOnID)
}
