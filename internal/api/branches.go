package api

import (
	"errors"
	"net/http"
	"os"

	"github.com/migueel15/dory/internal/app"
	"github.com/migueel15/dory/internal/workspace"
)

// mountBranches lists the local and remote branches of a project's checkout on any node.
func (s *Server) mountBranches(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/nodes/{node}/projects/{id}/branches", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		_, path, err := s.Projects.Checkout(r.Context(), r.PathValue("id"))
		if err != nil {
			writeErr(w, r, err)
			return
		}
		out := workspace.Branches{Branches: []string{}}
		if path != "" {
			_ = workspace.FetchOrigin(r.Context(), path)
			out = workspace.ListBranches(r.Context(), path)
		}
		writeJSON(w, http.StatusOK, out)
	}))
}

// mountNewThreadCommands lists the commands and skills a harness offers
// before its thread exists, for the new thread composer.
func (s *Server) mountNewThreadCommands(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/nodes/{node}/commands", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		var dir string
		if id := r.URL.Query().Get("project"); id != "" {
			_, path, err := s.Projects.Checkout(r.Context(), id)
			if err != nil && !errors.Is(err, app.ErrRepositoryNotCloned) {
				writeErr(w, r, err)
				return
			}
			dir = path
		}
		home, _ := os.UserHomeDir()
		if dir == "" {
			dir = home // only the user's configuration, never the server's folder
		}
		commands, skills := s.Runtime.Commands(s.Runtime.Paths(home), dir, r.URL.Query().Get("provider"))
		writeJSON(w, http.StatusOK, map[string]any{"commands": commands, "skills": skills})
	}))
}

// mountWorktrees lists the worktrees of a project's checkout on any node.
func (s *Server) mountWorktrees(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/nodes/{node}/projects/{id}/worktrees", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		_, path, err := s.Projects.Checkout(r.Context(), r.PathValue("id"))
		if err != nil {
			writeErr(w, r, err)
			return
		}
		out := []workspace.Worktree{}
		if path != "" {
			out = workspace.Worktrees(r.Context(), path)
		}
		for i := range out {
			if u, err := s.Runtime.WorktreeUse(r.Context(), out[i].Path, ""); err == nil {
				out[i].Threads, out[i].Working = u.Threads, u.Working
			}
		}
		writeJSON(w, http.StatusOK, out)
	}))
}
