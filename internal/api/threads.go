package api

import (
	"context"
	"crypto/rand"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/htmlrender"
	"github.com/migueel15/dory/internal/mesh"
	"github.com/migueel15/dory/internal/mesh/peerapi"
	"github.com/migueel15/dory/internal/runtime"
	"github.com/migueel15/dory/internal/workspace"
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
	mux.HandleFunc("GET /api/threads/{id}/services", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		writeJSON(w, http.StatusOK, s.Runtime.ThreadServices(t.ID))
	}))
	mux.HandleFunc("POST /api/threads/{id}/processes/{pid}/stop", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		pid, err := strconv.Atoi(r.PathValue("pid"))
		if err != nil || pid < 2 {
			writeErr(w, r, apperr.New(apperr.Invalid, "invalid PID").With("reason", "pid_invalid"))
			return
		}
		if err := s.Runtime.StopService(t.ID, pid); err != nil {
			writeErr(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("GET /api/threads", func(w http.ResponseWriter, r *http.Request) {
		f, err := parseFilter(r)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		if f.Limit == 0 {
			f.Limit = 1000
		}
		threads, err := s.Store.Threads().List(r.Context(), f)
		if err != nil {
			writeErr(w, r, err)
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
			writeErr(w, r, err)
			return
		}
		requests, err := s.Runtime.Requests(r.Context(), t.ID)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		dir, _ := s.Runtime.Dir(r.Context(), t.ID)
		writeJSON(w, http.StatusOK, ThreadView{Thread: t, LastTurnModel: s.Runtime.LastTurnModel(r.Context(), t.ID), ReportedSettings: s.Runtime.ReportedSettings(r.Context(), t), Messages: msgs, Queue: s.Runtime.Queue(t.ID), Requests: requests, Running: s.Runtime.Running(t.ID), Worktree: dir, Local: true})
	}))
	mux.HandleFunc("POST /api/threads/{id}/messages", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		var body peerapi.SendMessageRequest
		if !decode(w, r, &body) {
			return
		}
		if body.Text == "" && len(body.Files) == 0 {
			writeErr(w, r, apperr.New(apperr.Invalid, "text is required"))
			return
		}
		var files []runtime.Attachment
		for _, id := range body.Files {
			a, err := s.Runtime.Attachment(t.ID, id)
			if err != nil {
				writeErr(w, r, err)
				return
			}
			files = append(files, a)
		}
		if err := s.Runtime.SendFrom(r.Context(), t.ID, body.SenderThreadID, body.Text, files...); err != nil {
			writeErr(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	// A subagent's node reports how its turn ended; the parent's node records
	// it and wakes the parent.
	mux.HandleFunc("POST /api/threads/{id}/delegations/complete", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		var o runtime.Outcome
		if !decode(w, r, &o) {
			return
		}
		o.ParentThreadID = t.ID
		if err := s.Runtime.Complete(r.Context(), o); err != nil {
			writeErr(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("GET /api/threads/{id}/delegations", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		out, err := s.Runtime.Delegations(r.Context(), t.ID)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
	// Going back to a user message: files, conversation and session.
	mux.HandleFunc("POST /api/threads/{id}/revert", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		var body struct {
			MessageID string `json:"message_id"`
		}
		if !decode(w, r, &body) {
			return
		}
		out, err := s.Runtime.Revert(r.Context(), t.ID, body.MessageID)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
	mux.HandleFunc("POST /api/threads/{id}/interrupt", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		if err := s.Runtime.Interrupt(r.Context(), t.ID); err != nil {
			writeErr(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	// Answers to a request the agent waits for, one list per question.
	mux.HandleFunc("POST /api/threads/{id}/requests/{req}", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		var body struct {
			Answers [][]string `json:"answers"`
			// Files lists, per question, the attachments sent with its answer.
			Files [][]string `json:"files"`
		}
		if !decode(w, r, &body) {
			return
		}
		for i, ids := range body.Files {
			if i >= len(body.Answers) || len(ids) == 0 {
				continue
			}
			var files []runtime.Attachment
			for _, id := range ids {
				a, err := s.Runtime.Attachment(t.ID, id)
				if err != nil {
					writeErr(w, r, err)
					return
				}
				files = append(files, a)
			}
			// The paths go with the free answer, as in a message.
			a := body.Answers[i]
			if len(a) == 0 {
				a = []string{""}
			}
			a[len(a)-1] = runtime.WithAttachments(a[len(a)-1], files)
			body.Answers[i] = a
		}
		err := s.Runtime.Answer(r.Context(), t.ID, r.PathValue("req"), body.Answers)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	// The values of a secret request, or its decline. The body is never logged.
	mux.HandleFunc("POST /api/threads/{id}/secrets/{req}", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		var body struct {
			Values  map[string]string `json:"values"`
			Decline bool              `json:"decline"`
		}
		if !decode(w, r, &body) {
			return
		}
		err := s.Runtime.AnswerSecret(r.Context(), t.ID, r.PathValue("req"), body.Values, body.Decline)
		clear(body.Values)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST /api/threads/{id}/attachments", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		f, h, err := r.FormFile("file")
		if err != nil {
			writeErr(w, r, apperr.New(apperr.Invalid, "file is required"))
			return
		}
		defer f.Close()
		a, err := s.Runtime.SaveAttachment(t.ID, h.Filename, h.Header.Get("Content-Type"), f)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, a)
	}))
	mux.HandleFunc("GET /api/threads/{id}/attachments/{file}", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		path, err := s.Runtime.AttachmentPath(t.ID, r.PathValue("file"))
		if err != nil {
			writeErr(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "private, max-age=86400")
		http.ServeFile(w, r, path)
	}))
	// A page of render_html, wrapped in the theme. The CSP sandbox gives it an
	// opaque origin even when opened outside its iframe.
	mux.HandleFunc("GET /api/threads/{id}/renders/{file}", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		path, err := s.Runtime.AttachmentPath(t.ID, r.PathValue("file"))
		if err != nil {
			writeErr(w, r, err)
			return
		}
		page, err := os.ReadFile(path)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "sandbox allow-scripts allow-popups")
		w.Header().Set("Cache-Control", "private, max-age=86400")
		w.Write([]byte(htmlrender.Document(string(page), r.URL.Query().Get("theme") != "light")))
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
			writeErr(w, r, apperr.New(apperr.Invalid, "action must be edit, cancel or steer"))
			return
		}
		if err != nil {
			writeErr(w, r, err)
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
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
	// Slash commands and skills of the thread's harness, for / and $ in the composer.
	mux.HandleFunc("GET /api/threads/{id}/commands", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		dir, err := s.Runtime.Dir(r.Context(), t.ID)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		home, _ := os.UserHomeDir()
		commands, skills := s.Runtime.Commands(s.Runtime.Paths(home), dir, t.Provider)
		writeJSON(w, http.StatusOK, map[string]any{"commands": commands, "skills": skills})
	}))
	// Files of the thread's folder, for @ mentions in the composer.
	mux.HandleFunc("GET /api/threads/{id}/files", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		dir, err := s.Runtime.Dir(r.Context(), t.ID)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, runtime.Files(r.Context(), dir, r.URL.Query().Get("q"), 30))
	}))
	mux.HandleFunc("GET /api/threads/{id}/diff", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		d, err := s.Runtime.DiffAgainst(r.Context(), t.ID, r.URL.Query().Get("base"))
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"diff": d.Diff, "branch": t.Branch, "base": d.Base, "branches": d.Branches})
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
			writeErr(w, r, err)
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
			writeErr(w, r, err)
			return
		}
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

func (s *Server) createThread(w http.ResponseWriter, r *http.Request) {
	var body peerapi.CreateThreadRequest
	if !decode(w, r, &body) {
		return
	}
	ctx := r.Context()
	var path string
	if body.RepositoryID == "" && body.ProjectID != nil {
		var err error
		body.RepositoryID, path, err = s.projectCheckout(ctx, *body.ProjectID)
		if err != nil {
			writeErr(w, r, err)
			return
		}
	} else if body.RepositoryID != "" {
		var err error
		if path, err = s.Store.RepositoryPath(ctx, body.RepositoryID); errors.Is(err, data.ErrNotFound) {
			writeErr(w, r, apperr.New(apperr.RepositoryNotClone, "the repository is not cloned on this node"))
			return
		} else if err != nil {
			writeErr(w, r, err)
			return
		}
	}
	if body.Title == "" {
		body.Title = shortTitle(body.Text)
	}
	if body.WorktreePath != "" {
		// Only a worktree git knows for the project's repository is accepted.
		var found *workspace.Worktree
		if path != "" {
			for _, wt := range workspace.Worktrees(ctx, path) {
				if wt.Path == body.WorktreePath {
					found = &wt
					break
				}
			}
		}
		if found == nil {
			writeErr(w, r, apperr.New(apperr.UnknownWorktree, "the worktree does not belong to the project's repository"))
			return
		}
		body.Worktree, body.Branch = false, found.Branch
		body.BaseBranch = workspace.DefaultBranch(ctx, path)
		if u, err := s.Runtime.WorktreeUse(ctx, found.Path, ""); err == nil && u.BaseBranch != "" {
			body.BaseBranch = u.BaseBranch
		}
	} else if !body.Worktree && body.Branch != "" && path != "" {
		// The current checkout runs on the branch the user picked.
		if err := workspace.SwitchBranch(ctx, path, body.Branch); err != nil {
			writeErr(w, r, err)
			return
		}
	}
	defaults := s.Settings.Get().Threads
	if body.Provider == "" && defaults.DefaultModel.Provider != "" {
		body.Provider, body.Model, body.Effort = defaults.DefaultModel.Provider, defaults.DefaultModel.Model, defaults.DefaultModel.Effort
	}
	if body.WorktreePath == "" && body.Branch == "" && body.BaseBranch == "" && path != "" && defaults.DefaultWorkspace == "worktree" {
		body.Worktree = true
	}
	if body.Worktree && body.Branch == "" {
		// A new worktree gets its own branch; the agent may rename it later.
		if name := s.worktreeBranch(ctx, body.Title, body.Text); name != "" && !workspace.BranchExists(ctx, path, name) {
			body.Branch = name
		} else {
			body.Branch = "dory/" + strings.ToLower(rand.Text()[:8])
		}
	}
	nt := runtime.NewThread{ProjectID: body.ProjectID, TaskID: body.TaskID, ParentID: body.ParentID, OriginID: body.OriginID, Provider: body.Provider, Model: body.Model,
		Effort: body.Effort, RuntimeMode: body.RuntimeMode, Title: body.Title, Worktree: body.Worktree, RepoPath: path, Branch: body.Branch, BaseBranch: body.BaseBranch, ExistingDir: body.WorktreePath}
	t, err := s.Runtime.Create(ctx, nt)
	if fb := defaults.FallbackModel; err != nil && fb.Provider != "" && fb.Provider != nt.Provider {
		// The chosen model could not start: retry once with the node's fallback.
		nt.Provider, nt.Model, nt.Effort = fb.Provider, fb.Model, fb.Effort
		t, err = s.Runtime.Create(ctx, nt)
	}
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if body.Branch != "" && s.Settings.Get().SourceControl.AutoPull {
		// Start the thread on the latest commits of its branch.
		if dir, derr := s.Runtime.Dir(ctx, t.ID); derr == nil {
			if perr := workspace.Pull(ctx, dir, body.Branch); perr != nil {
				log.Printf("api: auto-pull %s: %v", body.Branch, perr)
			}
		}
	}
	if body.Text != "" {
		if err = s.Runtime.Send(ctx, t.ID, body.Text); err != nil {
			writeErr(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, peerapi.CreateThreadResponse{ThreadID: t.ID})
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
	if action == "read" {
		// The remote view may already show activity that has not reached this cache.
		if t.NodeID != s.Node.ID && !t.StateStale && !strings.HasPrefix(auth.Actor(ctx), "peer:") && s.PeerClient != nil {
			err := s.PeerClient.Call(ctx, t.NodeID, http.MethodPost, "/api/threads/"+url.PathEscape(id)+"/actions",
				map[string]string{"action": action}, &t)
			if !errors.Is(err, mesh.ErrUnreachable) {
				return t, err
			}
		}
		return s.Store.MarkThreadRead(ctx, id)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	fields := make(map[string]any)
	switch action {
	case "rename":
		if title == "" {
			return t, data.ErrInvalid.Msgf("title is required")
		}
		fields["title"] = title
	case "pin":
		fields["pinned_at"] = now
	case "unpin":
		fields["pinned_at"] = nil
	case "unread":
		fields["read_at"] = nil
	case "settle":
		fields["settled_at"] = now
	case "unsettle":
		fields["settled_at"] = nil
	case "snooze":
		if _, err := time.Parse(time.RFC3339, until); err != nil {
			return t, data.ErrInvalid.Msgf("until must be an RFC 3339 time")
		}
		fields["snoozed_until"] = until
	case "unsnooze":
		fields["snoozed_until"] = nil
	case "link":
		if taskID == "" {
			fields["task_id"] = nil
			fields["task_linked_at"] = nil
		} else {
			fields["task_id"] = taskID
			fields["task_linked_at"] = now
		}
	case "archive":
		return t, s.archiveThread(ctx, t)
	default:
		return t, data.ErrInvalid.Msgf("unknown action %s", action)
	}
	return s.Store.Threads().Patch(ctx, id, fields)
}
