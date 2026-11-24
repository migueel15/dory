package api

import (
	"net/http"
	"strconv"

	"github.com/migueel15/dory/internal/app"
	"github.com/migueel15/dory/internal/apperr"
)

// mountPulls serves the review aids of a pull request: files marked as
// reviewed on this node and the side chat with an agent (requirement RF-25).
func (s *Server) mountPulls(mux *http.ServeMux) {
	base := "/api/github/repos/{owner}/{repo}/pulls/{number}"
	target := func(w http.ResponseWriter, r *http.Request) (string, int, bool) {
		n, err := strconv.Atoi(r.PathValue("number"))
		if err != nil {
			writeErr(w, r, apperr.New(apperr.Invalid, "invalid number"))
			return "", 0, false
		}
		return r.PathValue("owner") + "/" + r.PathValue("repo"), n, true
	}
	mux.HandleFunc("GET "+base+"/reviews", func(w http.ResponseWriter, r *http.Request) {
		repo, n, ok := target(w, r)
		if !ok {
			return
		}
		marks, err := s.PullReviews.Marks(r.Context(), repo, n)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, marks)
	})
	mux.HandleFunc("PUT "+base+"/reviews", func(w http.ResponseWriter, r *http.Request) {
		repo, n, ok := target(w, r)
		if !ok {
			return
		}
		var body struct {
			Path      string `json:"path"`
			PatchHash string `json:"patch_hash"`
		}
		if !decode(w, r, &body) {
			return
		}
		if err := s.PullReviews.Mark(r.Context(), repo, n, body.Path, body.PatchHash); err != nil {
			writeErr(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE "+base+"/reviews", func(w http.ResponseWriter, r *http.Request) {
		repo, n, ok := target(w, r)
		if !ok {
			return
		}
		if err := s.PullReviews.Unmark(r.Context(), repo, n, r.URL.Query().Get("path")); err != nil {
			writeErr(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET "+base+"/agent", func(w http.ResponseWriter, r *http.Request) {
		repo, n, ok := target(w, r)
		if !ok {
			return
		}
		t, err := s.PullReviews.Thread(r.Context(), repo, n)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		if t == nil {
			writeJSON(w, http.StatusOK, map[string]any{"available": false})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"available": true, "thread_id": t.ID})
	})
	ask := func(w http.ResponseWriter, r *http.Request) {
		repo, n, ok := target(w, r)
		if !ok {
			return
		}
		var body app.PullQuestion
		if !decode(w, r, &body) {
			return
		}
		t, err := s.PullReviews.Ask(r.Context(), repo, n, body)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"available": true, "thread_id": t.ID})
	}
	mux.HandleFunc("POST "+base+"/ask", ask)
	mux.HandleFunc("POST "+base+"/agent", ask)
}
