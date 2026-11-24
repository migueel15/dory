package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"

	"github.com/migueel15/dory/internal/runtime"
)

// ExternalView is a harness session with what Dory knows about it.
type ExternalView struct {
	runtime.External
	ProjectID *string `json:"project_id"`
	ThreadID  string  `json:"thread_id,omitempty"`
}

// mountExternal lets the interface see the conversations started in the
// harnesses of any node, and continue them as Dory threads.
func (s *Server) mountExternal(mux *http.ServeMux) {
	home, _ := os.UserHomeDir()
	mux.HandleFunc("GET /api/nodes/{node}/external", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		adopted, err := s.Runtime.Adopted(r.Context())
		if err != nil {
			writeErr(w, r, err)
			return
		}
		out := []ExternalView{}
		for _, e := range s.Runtime.ListExternal(r.Context(), s.Runtime.Paths(home)) {
			out = append(out, ExternalView{External: e, ProjectID: s.projectOf(r.Context(), e.Dir), ThreadID: adopted[e.ID]})
		}
		writeJSON(w, http.StatusOK, out)
	}))
	mux.HandleFunc("GET /api/nodes/{node}/external/{provider}/{id}", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		e, err := s.Runtime.FindExternal(r.Context(), s.Runtime.Paths(home), r.PathValue("provider"), r.PathValue("id"))
		if err != nil {
			writeErr(w, r, err)
			return
		}
		msgs, err := s.Runtime.ReadExternal(r.Context(), e)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"session": e, "messages": msgs})
	}))
	mux.HandleFunc("POST /api/nodes/{node}/external/{provider}/{id}/adopt", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		e, err := s.Runtime.FindExternal(r.Context(), s.Runtime.Paths(home), r.PathValue("provider"), r.PathValue("id"))
		if err != nil {
			writeErr(w, r, err)
			return
		}
		t, err := s.Threads.Adopt(r.Context(), e, s.projectOf(r.Context(), e.Dir))
		if err != nil {
			writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"thread_id": t.ID})
	}))
}

// projectOf finds the project whose repository this node keeps at dir or above it.
func (s *Server) projectOf(ctx context.Context, dir string) *string {
	paths, err := s.Store.RepositoryPaths(ctx)
	if err != nil || dir == "" {
		return nil
	}
	for id, path := range paths {
		if rel, err := filepath.Rel(path, dir); err == nil && filepath.IsLocal(rel) {
			if r, err := s.Store.Repositories().Get(ctx, id); err == nil {
				return &r.ProjectID
			}
		}
	}
	return nil
}
