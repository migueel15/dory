package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/runtime"
)

// ThreadView is a thread with its conversation, as the chat shows it.
type ThreadView struct {
	data.Thread
	LastTurnModel    *runtime.TurnModel       `json:"last_turn_model,omitempty"`
	ReportedSettings runtime.ReportedSettings `json:"reported_settings"`
	Messages         []runtime.Message        `json:"messages"`
	Queue            []runtime.Queued         `json:"queue"`
	// Requests are what the agent waits for in the middle of its turn.
	Requests []runtime.Request `json:"requests"`
	Running  bool              `json:"running"`
	Worktree string            `json:"worktree"`
	// Local is false when another node owns the thread; its messages then
	// come from that node.
	Local bool `json:"local"`
}

// Remote reaches the node that owns a thread; set by the mesh.
type Remote interface {
	Forward(w http.ResponseWriter, r *http.Request, node string) bool
}

func (s *Server) mountThreads(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/threads", func(w http.ResponseWriter, r *http.Request) {
		f, err := parseFilter(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
			return
		}
		if f.Limit == 0 {
			f.Limit = 1000
		}
		threads, err := s.Store.Threads().List(r.Context(), f)
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, threads)
	})
	mux.HandleFunc("POST /api/threads", s.createThread)
	// A thread runs where it is created: the mesh forwards it to the chosen node.
	mux.HandleFunc("POST /api/nodes/{node}/threads", s.onNode(s.createThread))
	mux.HandleFunc("GET /api/threads/{id}", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		msgs, err := s.Runtime.Messages(r.Context(), t.ID)
		if err != nil {
			writeDataError(w, err)
			return
		}
		requests, err := s.Runtime.Requests(r.Context(), t.ID)
		if err != nil {
			writeDataError(w, err)
			return
		}
		dir, _ := s.Runtime.Dir(r.Context(), t.ID)
		writeJSON(w, http.StatusOK, ThreadView{Thread: t, LastTurnModel: s.Runtime.LastTurnModel(r.Context(), t.ID), ReportedSettings: s.Runtime.ReportedSettings(r.Context(), t), Messages: msgs, Queue: s.Runtime.Queue(t.ID), Requests: requests, Running: s.Runtime.Running(t.ID), Worktree: dir, Local: true})
	}))
	mux.HandleFunc("POST /api/threads/{id}/messages", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		var body struct {
			Text  string   `json:"text"`
			Files []string `json:"files"`
		}
		if !decode(w, r, &body) {
			return
		}
		if body.Text == "" && len(body.Files) == 0 {
			writeError(w, http.StatusBadRequest, "invalid_argument", "text is required")
			return
		}
		var files []runtime.Attachment
		for _, id := range body.Files {
			a, err := s.Runtime.Attachment(t.ID, id)
			if err != nil {
				writeDataError(w, err)
				return
			}
			files = append(files, a)
		}
		if err := s.Runtime.Send(r.Context(), t.ID, body.Text, files...); err != nil {
			writeDataError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST /api/threads/{id}/interrupt", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		if err := s.Runtime.Interrupt(r.Context(), t.ID); err != nil {
			writeDataError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	// Answers to a request the agent waits for, one list per question.
	mux.HandleFunc("POST /api/threads/{id}/requests/{req}", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		var body struct {
			Answers [][]string `json:"answers"`
		}
		if !decode(w, r, &body) {
			return
		}
		err := s.Runtime.Answer(r.Context(), t.ID, r.PathValue("req"), body.Answers)
		if errors.Is(err, runtime.ErrNoRequest) {
			writeError(w, http.StatusConflict, "conflict", err.Error())
			return
		}
		if err != nil {
			writeDataError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST /api/threads/{id}/attachments", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		f, h, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", "file is required")
			return
		}
		defer f.Close()
		a, err := s.Runtime.SaveAttachment(t.ID, h.Filename, h.Header.Get("Content-Type"), f)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, a)
	}))
	mux.HandleFunc("GET /api/threads/{id}/attachments/{file}", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		path, err := s.Runtime.AttachmentPath(t.ID, r.PathValue("file"))
		if err != nil {
			writeDataError(w, err)
			return
		}
		w.Header().Set("Cache-Control", "private, max-age=86400")
		http.ServeFile(w, r, path)
	}))
	// Messages waiting for the running turn: edit, drop or send now.
	mux.HandleFunc("POST /api/threads/{id}/queue/{run}", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		var body struct {
			Action string `json:"action"`
			Text   string `json:"text"`
		}
		if !decode(w, r, &body) {
			return
		}
		run := r.PathValue("run")
		var err error
		switch body.Action {
		case "edit":
			err = s.Runtime.EditQueued(r.Context(), t.ID, run, body.Text)
		case "cancel":
			err = s.Runtime.CancelQueued(r.Context(), t.ID, run)
		case "steer":
			err = s.Runtime.SteerQueued(r.Context(), t.ID, run)
		default:
			writeError(w, http.StatusBadRequest, "invalid_argument", "action must be edit, cancel or steer")
			return
		}
		if err != nil {
			writeDataError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	// Model, effort and permissions apply from the next turn.
	mux.HandleFunc("POST /api/threads/{id}/settings", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		var body runtime.ThreadSettings
		if !decode(w, r, &body) {
			return
		}
		out, err := s.Runtime.Configure(r.Context(), t.ID, body)
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
	// Slash commands and skills of the thread's harness, for / and $ in the composer.
	mux.HandleFunc("GET /api/threads/{id}/commands", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		dir, err := s.Runtime.Dir(r.Context(), t.ID)
		if err != nil {
			writeDataError(w, err)
			return
		}
		home, _ := os.UserHomeDir()
		commands, skills := runtime.Commands(home, dir, t.Provider)
		writeJSON(w, http.StatusOK, map[string]any{"commands": commands, "skills": skills})
	}))
	// Files of the thread's folder, for @ mentions in the composer.
	mux.HandleFunc("GET /api/threads/{id}/files", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		dir, err := s.Runtime.Dir(r.Context(), t.ID)
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runtime.Files(r.Context(), dir, r.URL.Query().Get("q"), 30))
	}))
	mux.HandleFunc("GET /api/threads/{id}/diff", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		diff, err := s.Runtime.Diff(r.Context(), t.ID)
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"diff": diff, "branch": t.Branch})
	}))
	// Organisation of a thread replicates, so any node can change it.
	mux.HandleFunc("POST /api/threads/{id}/actions", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Action string `json:"action"`
			Title  string `json:"title"`
			Until  string `json:"until"`
			TaskID string `json:"task_id"`
		}
		if !decode(w, r, &body) {
			return
		}
		t, err := s.ThreadAction(r.Context(), r.PathValue("id"), body.Action, body.Title, body.Until, body.TaskID)
		if err != nil {
			writeDataError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, t)
	})
}

// owned runs h on the node that owns the thread: here, or forwarded to its
// node through the mesh.
func (s *Server) owned(h func(http.ResponseWriter, *http.Request, data.Thread)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := s.Store.Threads().Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeDataError(w, err)
			return
		}
		if t.NodeID != s.Node.ID {
			if s.Remote != nil && s.Remote.Forward(w, r, t.NodeID) {
				return
			}
			writeError(w, http.StatusServiceUnavailable, "node_offline", "el nodo que ejecuta este hilo no está conectado")
			return
		}
		h(w, r, t)
	}
}

func (s *Server) createThread(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProjectID    *string `json:"project_id"`
		TaskID       *string `json:"task_id"`
		RepositoryID string  `json:"repository_id"`
		Provider     string  `json:"provider"`
		Model        string  `json:"model"`
		Effort       string  `json:"effort"`
		RuntimeMode  string  `json:"runtime_mode"`
		Title        string  `json:"title"`
		Text         string  `json:"text"`
		Worktree     bool    `json:"worktree"`
		Branch       string  `json:"branch"`
		BaseBranch   string  `json:"base_branch"`
	}
	if !decode(w, r, &body) {
		return
	}
	ctx := r.Context()
	if body.RepositoryID == "" && body.ProjectID != nil {
		repos, _ := s.Store.Repositories().List(ctx, data.Filter{Fields: map[string]any{"project_id": *body.ProjectID}})
		if len(repos) > 0 {
			body.RepositoryID = repos[0].ID
		}
	}
	var path string
	if body.RepositoryID != "" {
		var err error
		if path, err = s.Store.RepositoryPath(ctx, body.RepositoryID); errors.Is(err, data.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "repository_not_cloned", "el repositorio no está clonado en este nodo")
			return
		} else if err != nil {
			writeDataError(w, err)
			return
		}
	}
	if body.Title == "" {
		body.Title = shortTitle(body.Text)
	}
	t, err := s.Runtime.Create(ctx, runtime.NewThread{ProjectID: body.ProjectID, TaskID: body.TaskID, Provider: body.Provider, Model: body.Model,
		Effort: body.Effort, RuntimeMode: body.RuntimeMode, Title: body.Title, Worktree: body.Worktree, RepoPath: path, Branch: body.Branch, BaseBranch: body.BaseBranch})
	if err != nil {
		writeDataError(w, err)
		return
	}
	if body.Text != "" {
		if err = s.Runtime.Send(ctx, t.ID, body.Text); err != nil {
			writeDataError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"thread_id": t.ID})
}

func shortTitle(text string) string {
	r := []rune(text)
	for i, c := range r {
		if c == '\n' {
			r = r[:i]
			break
		}
	}
	if len(r) > 60 {
		r = append(r[:59], '…')
	}
	if len(r) == 0 {
		return "Nuevo hilo"
	}
	return string(r)
}

func (s *Server) ThreadAction(ctx context.Context, id, action, title, until, taskID string) (data.Thread, error) {
	t, err := s.Store.Threads().Get(ctx, id)
	if err != nil {
		return t, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	fields := make(map[string]any)
	switch action {
	case "rename":
		if title == "" {
			return t, errors.Join(data.ErrInvalid, errors.New("title is required"))
		}
		fields["title"] = title
	case "pin":
		fields["pinned_at"] = now
	case "unpin":
		fields["pinned_at"] = nil
	case "read":
		fields["read_at"] = now
	case "unread":
		fields["read_at"] = nil
	case "settle":
		fields["settled_at"] = now
	case "unsettle":
		fields["settled_at"] = nil
	case "snooze":
		if _, err := time.Parse(time.RFC3339, until); err != nil {
			return t, errors.Join(data.ErrInvalid, errors.New("until must be an RFC 3339 time"))
		}
		fields["snoozed_until"] = until
	case "unsnooze":
		fields["snoozed_until"] = nil
	case "link":
		if taskID == "" {
			fields["task_id"] = nil
		} else {
			fields["task_id"] = taskID
		}
	case "archive":
		if t.NodeID == s.Node.ID {
			if err := s.Runtime.Remove(ctx, id); err != nil {
				return t, err
			}
		}
		return t, s.Store.Threads().Archive(ctx, id)
	default:
		return t, errors.Join(data.ErrInvalid, errors.New("unknown action "+action))
	}
	return s.Store.PatchThread(ctx, id, fields)
}
