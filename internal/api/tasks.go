package api

import (
	"context"
	"net/http"

	"github.com/migueel15/dory/internal/app"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/domain/board"
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
		deps, err := s.Store.Dependencies().ListAll(r.Context(), data.Filter{Fields: map[string]any{"task_id": r.PathValue("id")}})
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
	mux.HandleFunc("GET /api/tasks/{id}/plan", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.Plans.Document(r.Context(), r.PathValue("id"))
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	})
	mux.HandleFunc("PUT /api/tasks/{id}/plan", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			BodyMD   string `json:"body_md"`
			Revision int64  `json:"revision"`
		}
		if !decode(w, r, &body) {
			return
		}
		p, err := s.Plans.Write(r.Context(), r.PathValue("id"), body.BodyMD, body.Revision, "user")
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	})
	mux.HandleFunc("GET /api/tasks/{id}/plans", func(w http.ResponseWriter, r *http.Request) {
		plans, err := s.Plans.List(r.Context(), r.PathValue("id"))
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, plans)
	})
	mux.HandleFunc("POST /api/tasks/{id}/plan/accept", func(w http.ResponseWriter, r *http.Request) {
		t, p, err := s.Plans.Accept(r.Context(), r.PathValue("id"))
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"task": t, "plan": p})
	})
	mux.HandleFunc("POST /api/tasks/{id}/launch", s.launch)
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

// launch starts an implementation thread for a task on this node or on the
// chosen peer, in its own worktree and branch.
func (s *Server) launch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Node        string `json:"node"`
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
	threadID, branch, err := s.Tasks.Launch(r.Context(), r.PathValue("id"), app.LaunchInput{Node: body.Node, Provider: body.Provider, Model: body.Model,
		Effort: body.Effort, RuntimeMode: body.RuntimeMode, BaseBranch: body.BaseBranch, Text: body.Text})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"thread_id": threadID, "branch": branch})
}
