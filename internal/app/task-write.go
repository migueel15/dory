package app

import (
	"context"
	"encoding/json"

	"github.com/migueel15/dory/internal/data"
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
	stage, hasStage := fields["stage"]
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
	return out, err
}

func invalidTaskStage() error {
	return data.ErrInvalid.Msgf("stage must be inbox, ready, done or cancelled")
}
