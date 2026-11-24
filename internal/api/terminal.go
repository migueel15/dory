package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/coder/websocket"
	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
)

// mountTerminal serves the shells of each thread on the node that runs it.
func (s *Server) mountTerminal(mux *http.ServeMux) {
	need := func(w http.ResponseWriter, r *http.Request) bool {
		if s.Terminals == nil {
			writeErr(w, r, apperr.New(apperr.Unavailable, "this node has no terminals"))
			return false
		}
		return true
	}
	mux.HandleFunc("GET /api/threads/{id}/terminals", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		if need(w, r) {
			writeJSON(w, http.StatusOK, s.Terminals.List(t.ID))
		}
	}))
	mux.HandleFunc("POST /api/threads/{id}/terminals", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		var body struct {
			Name string `json:"name"`
			Cols int    `json:"cols"`
			Rows int    `json:"rows"`
		}
		if !decode(w, r, &body) || !need(w, r) {
			return
		}
		info, err := s.Terminals.Open(r.Context(), t.ID, body.Name, body.Cols, body.Rows)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, info)
	}))
	mux.HandleFunc("PATCH /api/threads/{id}/terminals/{tid}", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		var body struct {
			Name string `json:"name"`
		}
		if !decode(w, r, &body) || !need(w, r) {
			return
		}
		info, err := s.Terminals.Rename(t.ID, r.PathValue("tid"), body.Name)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, info)
	}))
	mux.HandleFunc("DELETE /api/threads/{id}/terminals/{tid}", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		if !need(w, r) {
			return
		}
		if _, err := s.Terminals.Close(t.ID, r.PathValue("tid")); err != nil {
			writeErr(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	// The socket carries the raw output to the client as binary frames; the
	// client sends keys as binary frames and {"type":"resize"} as text.
	mux.HandleFunc("GET /api/threads/{id}/terminals/{tid}/ws", s.owned(func(w http.ResponseWriter, r *http.Request, t data.Thread) {
		if !need(w, r) {
			return
		}
		sess, err := s.Terminals.Get(t.ID, r.PathValue("tid"))
		if err != nil {
			writeErr(w, r, err)
			return
		}
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer ws.CloseNow()
		ws.SetReadLimit(1 << 20)
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		first, out, unsubscribe := sess.Subscribe()
		defer unsubscribe()
		if len(first) > 0 {
			if ws.Write(ctx, websocket.MessageBinary, first) != nil {
				return
			}
		}
		go func() {
			defer cancel()
			for {
				kind, p, err := ws.Read(ctx)
				if err != nil {
					return
				}
				if kind == websocket.MessageBinary {
					if sess.Write(p) != nil {
						return
					}
					continue
				}
				var msg struct {
					Type string `json:"type"`
					Cols int    `json:"cols"`
					Rows int    `json:"rows"`
				}
				if json.Unmarshal(p, &msg) == nil && msg.Type == "resize" {
					_ = sess.Resize(msg.Cols, msg.Rows)
				}
			}
		}()
		for {
			select {
			case p, ok := <-out:
				if !ok {
					ws.Close(websocket.StatusNormalClosure, "exited")
					return
				}
				if ws.Write(ctx, websocket.MessageBinary, p) != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}))
}

// TerminalCwd is where the shell of a thread starts: its workspace, else the
// path of its project on this node, else nothing (the home folder).
func (s *Server) TerminalCwd(ctx context.Context, threadID string) string {
	if dir, err := s.Runtime.Dir(ctx, threadID); err == nil && dir != "" {
		return dir
	}
	t, err := s.Store.Threads().Get(ctx, threadID)
	if err != nil || t.ProjectID == nil {
		return ""
	}
	repo, err := s.Store.ProjectRepository(ctx, *t.ProjectID)
	if err != nil || repo == "" {
		return ""
	}
	path, _ := s.Store.RepositoryPath(ctx, repo)
	return path
}
