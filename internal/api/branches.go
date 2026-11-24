package api

import (
	"context"
	"errors"
	"github.com/migueel15/dory/internal/apperr"
	"net/http"
	"os"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/runtime"
	"github.com/migueel15/dory/internal/workspace"
)

// errNotCloned says the repository of a project has no checkout on this node.
var errNotCloned = apperr.New(apperr.RepositoryNotClone, "the repository is not cloned on this node")

// projectCheckout finds the repository a thread of the project works in and
// where this node keeps it. With several repositories, the first one is used.
func (s *Server) projectCheckout(ctx context.Context, projectID string) (repositoryID, path string, err error) {
	repos, err := s.Store.Repositories().List(ctx, data.Filter{Fields: map[string]any{"project_id": projectID}})
	if err != nil || len(repos) == 0 {
		return "", "", err
	}
	path, err = s.Store.RepositoryPath(ctx, repos[0].ID)
	if errors.Is(err, data.ErrNotFound) {
		return repos[0].ID, "", errNotCloned
	}
	return repos[0].ID, path, err
}

// mountBranches lists the local branches of a project's checkout on any node.
func (s *Server) mountBranches(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/nodes/{node}/projects/{id}/branches", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		_, path, err := s.projectCheckout(r.Context(), r.PathValue("id"))
		if errors.Is(err, errNotCloned) {
			writeErr(w, r, err)
			return
		}
		if err != nil {
			writeErr(w, r, err)
			return
		}
		out := workspace.Branches{Branches: []string{}}
		if path != "" {
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
			_, path, err := s.projectCheckout(r.Context(), id)
			if err != nil && !errors.Is(err, errNotCloned) {
				writeErr(w, r, err)
				return
			}
			dir = path
		}
		home, _ := os.UserHomeDir()
		if dir == "" {
			dir = home // only the user's configuration, never the server's folder
		}
		commands, skills := runtime.Commands(s.Runtime.Paths(home), dir, r.URL.Query().Get("provider"))
		writeJSON(w, http.StatusOK, map[string]any{"commands": commands, "skills": skills})
	}))
}

// mountWorktrees lists the worktrees of a project's checkout on any node.
func (s *Server) mountWorktrees(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/nodes/{node}/projects/{id}/worktrees", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		_, path, err := s.projectCheckout(r.Context(), r.PathValue("id"))
		if errors.Is(err, errNotCloned) {
			writeErr(w, r, err)
			return
		}
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

// ProjectDir is the local checkout of a project's first repository, or "" for
// a project without repositories.
func (s *Server) ProjectDir(ctx context.Context, projectID string) (string, error) {
	_, path, err := s.projectCheckout(ctx, projectID)
	return path, err
}
