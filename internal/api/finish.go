package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
)

var stages = map[string]bool{"inbox": true, "ready": true, "done": true, "cancelled": true}

// CreateTask validates the initial stage and applies its side effects.
func (s *Server) CreateTask(ctx context.Context, t data.Task) (data.Task, error) {
	if t.Stage != "" && !stages[t.Stage] {
		return t, invalidStage()
	}
	if t.Stage == "done" {
		if err := s.Tasks.CheckDone(ctx, t); err != nil {
			return t, err
		}
	}
	out, err := s.Store.Tasks().Create(ctx, t)
	if err == nil && (out.Stage == "done" || out.Stage == "cancelled") {
		s.Tasks.FinishTask(ctx, out.ID)
	}
	return out, err
}

// PatchTask checks stage transitions and applies them against the version
// used for the check, so a concurrent edit cannot invalidate the decision.
func (s *Server) PatchTask(ctx context.Context, id string, expected *string, fields data.Fields) (data.Task, error) {
	tasks := s.Store.Tasks()
	stage, hasStage := fields["stage"]
	if !hasStage {
		if expected != nil {
			return tasks.PatchIf(ctx, id, *expected, fields)
		}
		return tasks.Patch(ctx, id, fields)
	}
	next, ok := stage.(string)
	if !ok || !stages[next] {
		return data.Task{}, invalidStage()
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
		if err := s.Tasks.CheckDone(ctx, candidate); err != nil {
			return current, err
		}
	}
	out, err := tasks.PatchIf(ctx, id, current.UpdatedAt, fields)
	if err == nil && changed && (out.Stage == "done" || out.Stage == "cancelled") {
		s.Tasks.FinishTask(ctx, out.ID)
	}
	return out, err
}

func invalidStage() error {
	return data.ErrInvalid.Msgf("stage must be inbox, ready, done or cancelled")
}

// mountWorktreeState lets the interface warn about the worktree an archived
// thread kept and delete it on request.
func (s *Server) mountWorktreeState(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/nodes/{node}/kept-worktrees", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		kept, err := s.Runtime.KeptWorktrees(r.Context())
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, kept)
	}))
	mux.HandleFunc("DELETE /api/threads/{id}/worktree", s.ownedArchived(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		if err := s.Runtime.Remove(r.Context(), t.ID, true); err != nil {
			writeErr(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

// ownedArchived is owned for threads that may be archived.
func (s *Server) ownedArchived(h func(http.ResponseWriter, *http.Request, data.Thread)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		found, err := s.Store.Threads().List(r.Context(), data.Filter{Fields: map[string]any{"id": r.PathValue("id")}, IncludeArchived: true, Limit: 1})
		if err != nil {
			writeErr(w, r, err)
			return
		}
		if len(found) == 0 {
			writeErr(w, r, data.ErrNotFound)
			return
		}
		t := found[0]
		if t.NodeID != s.Node.ID {
			if s.Remote != nil && s.Remote.Forward(w, r, t.NodeID) {
				return
			}
			writeErr(w, r, apperr.New(apperr.NodeOffline, "the node that runs this thread is not connected").With("thread", true))
			return
		}
		h(w, r, t)
	}
}
