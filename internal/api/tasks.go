package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/migueel15/dory/internal/app"
	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/domain/board"
	"github.com/migueel15/dory/internal/mesh/peerapi"
)

// Card is a task as the kanban shows it.
type Card = app.Card

// Columns of the kanban, in order.
var Columns = func() []string {
	out := []string{}
	for _, c := range board.Columns() {
		out = append(out, string(c))
	}
	return out
}()

func (s *Server) mountTasks(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/board", s.board)
	mux.HandleFunc("GET /api/tasks/{id}/card", func(w http.ResponseWriter, r *http.Request) {
		c, err := s.Tasks.Card(r.Context(), r.PathValue("id"))
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, c)
	})
	mux.HandleFunc("GET /api/tasks/{id}/dependencies", func(w http.ResponseWriter, r *http.Request) {
		deps, err := s.Store.Dependencies().List(r.Context(), data.Filter{Fields: map[string]any{"task_id": r.PathValue("id")}, Limit: 1000})
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, deps)
	})
	mux.HandleFunc("PUT /api/tasks/{id}/dependencies/{dep}", func(w http.ResponseWriter, r *http.Request) {
		d, err := s.Store.Dependencies().Upsert(r.Context(), data.Dependency{TaskID: r.PathValue("id"), DependsOnID: r.PathValue("dep")})
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, d)
	})
	mux.HandleFunc("DELETE /api/tasks/{id}/dependencies/{dep}", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Store.Dependencies().Archive(r.Context(), r.PathValue("id")+"/"+r.PathValue("dep")); err != nil {
			writeErr(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.planRoutes(mux)
	mux.HandleFunc("GET /api/tasks/{id}/plans", func(w http.ResponseWriter, r *http.Request) {
		plans, err := s.Store.Plans().List(r.Context(), data.Filter{Fields: map[string]any{"task_id": r.PathValue("id")}, Limit: 1000})
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, plans)
	})
	mux.HandleFunc("POST /api/tasks/{id}/plan/accept", func(w http.ResponseWriter, r *http.Request) {
		t, p, err := s.acceptPlan(r.Context(), r.PathValue("id"))
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"task": t, "plan": p})
	})
	mux.HandleFunc("POST /api/tasks/{id}/launch", s.launch)
}

// planPending says whether the latest plan of a task waits for the user to
// accept it. A task without a plan is not pending.
func (s *Server) planPending(ctx context.Context, taskID string) (bool, error) {
	plans, err := s.Store.Plans().List(ctx, data.Filter{Fields: map[string]any{"task_id": taskID}, Limit: 1000})
	if err != nil || len(plans) == 0 {
		return false, err
	}
	latest := plans[0]
	for _, p := range plans {
		if p.Version > latest.Version {
			latest = p
		}
	}
	return latest.AcceptedAt == nil, nil
}

// acceptPlan marks the latest plan accepted and the task ready.
func (s *Server) acceptPlan(ctx context.Context, taskID string) (data.Task, data.Plan, error) {
	planMu.Lock()
	defer planMu.Unlock()
	latest, ok, err := s.latestPlan(ctx, taskID)
	if err != nil {
		return data.Task{}, data.Plan{}, err
	}
	if !ok {
		return data.Task{}, data.Plan{}, data.ErrInvalid.Msgf("the task has no plan")
	}
	return s.Store.AcceptPlan(ctx, latest)
}

func (s *Server) board(w http.ResponseWriter, r *http.Request) {
	f := data.Filter{}
	if p := r.URL.Query().Get("project"); p != "" {
		f.Fields = map[string]any{"project_id": p}
	}
	cards, err := s.Tasks.Board(r.Context(), f)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"columns": Columns, "cards": cards})
}

// Cards adds to each task its threads, pull request, blockers and column.
func (s *Server) Cards(ctx context.Context, tasks []data.Task) ([]Card, error) {
	return s.Tasks.Cards(ctx, tasks)
}

// launch starts an implementation thread for a task on this node, in its own
// worktree and branch.
func (s *Server) launch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider    string `json:"provider"`
		Model       string `json:"model"`
		Effort      string `json:"effort"`
		RuntimeMode string `json:"runtime_mode"`
		BaseBranch  string `json:"base_branch"`
		Text        string `json:"text"`
	}
	if !decode(w, r, &body) {
		return
	}
	ctx := r.Context()
	t, err := s.Store.Tasks().Get(ctx, r.PathValue("id"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if pending, err := s.planPending(ctx, t.ID); err != nil {
		writeErr(w, r, err)
		return
	} else if pending {
		writeErr(w, r, apperr.New(apperr.PlanNotAccepted, "accept the task plan before implementing it"))
		return
	}
	fields := data.Fields{}
	if t.RepositoryID == nil && t.ProjectID != nil {
		repos, _ := s.Store.Repositories().List(ctx, data.Filter{Fields: map[string]any{"project_id": *t.ProjectID}})
		if len(repos) == 1 {
			t.RepositoryID = &repos[0].ID
			fields["repository_id"] = repos[0].ID
		}
	}
	if t.RepositoryID == nil {
		writeErr(w, r, apperr.New(apperr.NoRepository, "the task needs a repository"))
		return
	}
	if _, _, err := app.RepositoryCheckout(ctx, s.Store, t.ProjectID, *t.RepositoryID); err != nil {
		writeErr(w, r, err)
		return
	}
	if t.Branch == "" {
		t.Branch = app.TaskBranch(t)
		fields["branch"] = t.Branch
	}
	if body.BaseBranch == "" {
		repo, _ := s.Store.Repositories().Get(ctx, *t.RepositoryID)
		body.BaseBranch = repo.DefaultBranch
	}
	if t, err = s.Store.Tasks().PatchIf(ctx, t.ID, t.UpdatedAt, fields); err != nil {
		writeErr(w, r, err)
		return
	}
	prompt := body.Text
	if prompt == "" {
		prompt = s.taskPrompt(ctx, t)
	}
	worktree := true
	th, err := s.Threads.Create(ctx, app.CreateThread{CreateThreadRequest: peerapi.CreateThreadRequest{
		ProjectID: t.ProjectID, RepositoryID: *t.RepositoryID, TaskID: &t.ID, Provider: body.Provider, Model: body.Model,
		Effort: body.Effort, RuntimeMode: body.RuntimeMode, Title: t.Title, Text: prompt, Worktree: &worktree, Branch: t.Branch, BaseBranch: body.BaseBranch,
	}})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"thread_id": th.ID, "branch": t.Branch})
}

// taskPrompt is the first message of an implementation thread.
func (s *Server) taskPrompt(ctx context.Context, t data.Task) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Implementa esta tarea de Dory (id %s) en la rama %s.\n\n# %s\n\n%s\n", t.ID, t.Branch, t.Title, t.Description)
	issue := ""
	if t.IssueNumber != nil {
		issue = fmt.Sprint(*t.IssueNumber)
		fmt.Fprintf(&b, "\nIssue de GitHub: #%d.\n", *t.IssueNumber)
	}
	plans, _ := s.Store.Plans().List(ctx, data.Filter{Fields: map[string]any{"task_id": t.ID}})
	var accepted *data.Plan
	for i := range plans {
		if plans[i].AcceptedAt != nil && (accepted == nil || plans[i].Version > accepted.Version) {
			accepted = &plans[i]
		}
	}
	if accepted != nil {
		fmt.Fprintf(&b, "\nPlan aceptado:\n\n%s\n", planMarkdown(*accepted))
	}
	return autoMessage("implement", map[string]string{"task": t.ID, "title": t.Title, "branch": t.Branch, "issue": issue}, b.String(), "")
}
