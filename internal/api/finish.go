package api

import (
	"net/http"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
)

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
