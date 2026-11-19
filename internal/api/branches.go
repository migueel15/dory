package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/migueel15/dory/internal/cmdexec"
	"github.com/migueel15/dory/internal/data"
)

// errNotCloned says the repository of a project has no checkout on this node.
var errNotCloned = errors.New("el repositorio no está clonado en este nodo")

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

// Branches is what a new thread can start from in a checkout of a node.
type Branches struct {
	Current  string   `json:"current"`
	Branches []string `json:"branches"`
}

// mountBranches lists the local branches of a project's checkout on any node.
func (s *Server) mountBranches(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/nodes/{node}/projects/{id}/branches", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		_, path, err := s.projectCheckout(r.Context(), r.PathValue("id"))
		if errors.Is(err, errNotCloned) {
			writeError(w, http.StatusBadRequest, "repository_not_cloned", err.Error())
			return
		}
		if err != nil {
			writeDataError(w, err)
			return
		}
		out := Branches{Branches: []string{}}
		if path != "" {
			out = branches(r.Context(), path)
		}
		writeJSON(w, http.StatusOK, out)
	}))
}

func branches(ctx context.Context, dir string) Branches {
	out := Branches{Branches: []string{}}
	if b, err := cmdexec.CommandContext(ctx, "git", "-C", dir, "branch", "--show-current").Output(); err == nil {
		out.Current = strings.TrimSpace(string(b))
	}
	if b, err := cmdexec.CommandContext(ctx, "git", "-C", dir, "for-each-ref", "--sort=-committerdate", "--format=%(refname:short)", "refs/heads").Output(); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			if line != "" {
				out.Branches = append(out.Branches, line)
			}
		}
	}
	return out
}

// switchBranch moves a checkout to branch before a thread starts in it. Git
// refuses when local changes would be lost; its message is returned as is.
func switchBranch(ctx context.Context, dir, branch string) error {
	if branches(ctx, dir).Current == branch {
		return nil
	}
	if out, err := cmdexec.CommandContext(ctx, "git", "-C", dir, "switch", branch).CombinedOutput(); err != nil {
		return fmt.Errorf("git switch %s: %s", branch, strings.TrimSpace(string(out)))
	}
	return nil
}
