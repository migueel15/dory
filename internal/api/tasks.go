package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/migueel15/dory/internal/app"
	"github.com/migueel15/dory/internal/apperr"
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
		d, err := s.Tasks.AddDependency(r.Context(), r.PathValue("id"), r.PathValue("dep"))
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
	mux.HandleFunc("PATCH /api/tasks/{id}/stage", s.moveTaskStage)
	mux.HandleFunc("POST /api/tasks/{id}/state", s.setTaskState)
	mux.HandleFunc("GET /api/tasks/{id}/comments", s.listTaskComments)
	mux.HandleFunc("POST /api/tasks/{id}/comments", s.commentTask)
	mux.HandleFunc("POST /api/tasks/{id}/attachments", func(w http.ResponseWriter, r *http.Request) {
		f, h, err := r.FormFile("file")
		if err != nil {
			writeErr(w, r, apperr.New(apperr.Invalid, "file is required"))
			return
		}
		defer f.Close()
		a, err := s.Runtime.SaveTaskAttachment(r.PathValue("id"), h.Filename, h.Header.Get("Content-Type"), f)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, a)
	})
	mux.HandleFunc("GET /api/tasks/{id}/attachments/{file}", func(w http.ResponseWriter, r *http.Request) {
		path, err := s.Runtime.TaskAttachmentPath(r.PathValue("id"), r.PathValue("file"))
		if err != nil {
			writeErr(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, max-age=86400")
		http.ServeFile(w, r, path)
	})
	mux.HandleFunc("GET /api/threads/{id}/pulls", s.listThreadPulls)
	mux.HandleFunc("POST /api/threads/{id}/pulls", s.linkThreadPull)
	mux.HandleFunc("DELETE /api/threads/{id}/pulls/{repository_id}/{number}", s.unlinkThreadPull)
}

func (s *Server) board(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	viewer := ""
	if s.GitHub != nil {
		viewer, _ = s.GitHub.Viewer(r.Context())
	}
	cards, err := s.Tasks.BoardMulti(r.Context(), app.TaskFilter{
		ProjectID: q.Get("project"),
		Source:    q.Get("source"),
		Scope:     q.Get("scope"),
		Open:      q.Get("open") == "true" || q.Get("open") == "1",
		Viewer:    viewer,
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"columns": Columns, "cards": cards, "viewer": viewer})
}

func (s *Server) emitBoardChanged() {
	if s.Events != nil {
		s.Events.Changed("board", "")
	}
}

func (s *Server) moveTaskStage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Stage string `json:"stage"`
	}
	if !decode(w, r, &body) {
		return
	}
	t, err := s.Tasks.Patch(r.Context(), r.PathValue("id"), nil, data.Fields{"stage": body.Stage})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	s.emitBoardChanged()
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) setTaskState(w http.ResponseWriter, r *http.Request) {
	var body struct {
		State  string `json:"state"`
		Reason string `json:"reason"`
	}
	if !decode(w, r, &body) {
		return
	}
	u, err := s.Tasks.SetState(r.Context(), r.PathValue("id"), app.TaskStateInput{State: body.State, Reason: body.Reason})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	s.emitBoardChanged()
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) listTaskComments(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, _, err := s.Tasks.ProviderFor(r.Context(), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	comments, err := p.ListComments(r.Context(), id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, comments)
}

func (s *Server) commentTask(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Body string `json:"body"`
	}
	if !decode(w, r, &body) {
		return
	}
	c, err := s.Tasks.Comment(r.Context(), r.PathValue("id"), body.Body)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	s.emitBoardChanged()
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) listThreadPulls(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ThreadPulls().ListByThread(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) linkThreadPull(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RepositoryID string `json:"repository_id"`
		Number       int64  `json:"number"`
		Branch       string `json:"branch"`
		LinkedBy     string `json:"linked_by"`
	}
	if !decode(w, r, &body) {
		return
	}
	tp, err := s.Store.ThreadPulls().Link(r.Context(), r.PathValue("id"), body.RepositoryID, body.Number, body.Branch, body.LinkedBy)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	s.emitBoardChanged()
	writeJSON(w, http.StatusCreated, tp)
}

func (s *Server) unlinkThreadPull(w http.ResponseWriter, r *http.Request) {
	num, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
	if err != nil || num <= 0 {
		writeErr(w, r, data.ErrInvalid.Msgf("invalid pull number"))
		return
	}
	if err := s.Store.ThreadPulls().Unlink(r.Context(), r.PathValue("id"), r.PathValue("repository_id"), num); err != nil {
		writeErr(w, r, err)
		return
	}
	s.emitBoardChanged()
	w.WriteHeader(http.StatusNoContent)
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
	threadID, branch, err := s.Tasks.Launch(r.Context(), r.PathValue("id"), app.LaunchInput{Provider: body.Provider, Model: body.Model,
		Effort: body.Effort, RuntimeMode: body.RuntimeMode, BaseBranch: body.BaseBranch, Text: body.Text})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"thread_id": threadID, "branch": branch})
}
