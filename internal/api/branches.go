package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/migueel15/dory/internal/cmdexec"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/runtime"
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

// mountNewThreadCommands lists the commands and skills a harness offers
// before its thread exists, for the new thread composer.
func (s *Server) mountNewThreadCommands(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/nodes/{node}/commands", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		var dir string
		if id := r.URL.Query().Get("project"); id != "" {
			_, path, err := s.projectCheckout(r.Context(), id)
			if err != nil && !errors.Is(err, errNotCloned) {
				writeDataError(w, err)
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

// Worktree is a worktree of a project's repository a new thread can join.
type Worktree struct {
	Path    string `json:"path"`
	Branch  string `json:"branch"`
	Threads int    `json:"threads"`
	Working int    `json:"working"`
}

// worktrees lists the worktrees of the repository at repo, without the main
// checkout.
func worktrees(ctx context.Context, repo string) []Worktree {
	out := []Worktree{}
	b, err := cmdexec.CommandContext(ctx, "git", "-C", repo, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return out
	}
	for i, block := range strings.Split(strings.TrimSpace(string(b)), "\n\n") {
		if i == 0 {
			continue // the main checkout
		}
		var w Worktree
		for _, line := range strings.Split(block, "\n") {
			if p, ok := strings.CutPrefix(line, "worktree "); ok {
				w.Path = p
			} else if br, ok := strings.CutPrefix(line, "branch "); ok {
				w.Branch = strings.TrimPrefix(br, "refs/heads/")
			}
		}
		if w.Path != "" {
			out = append(out, w)
		}
	}
	return out
}

// defaultBranch is the branch origin points HEAD to, or main.
func defaultBranch(ctx context.Context, repo string) string {
	if b, err := cmdexec.CommandContext(ctx, "git", "-C", repo, "symbolic-ref", "--short", "refs/remotes/origin/HEAD").Output(); err == nil {
		return strings.TrimPrefix(strings.TrimSpace(string(b)), "origin/")
	}
	return "main"
}

// mountWorktrees lists the worktrees of a project's checkout on any node.
func (s *Server) mountWorktrees(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/nodes/{node}/projects/{id}/worktrees", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		_, path, err := s.projectCheckout(r.Context(), r.PathValue("id"))
		if errors.Is(err, errNotCloned) {
			writeError(w, http.StatusBadRequest, "repository_not_cloned", err.Error())
			return
		}
		if err != nil {
			writeDataError(w, err)
			return
		}
		out := []Worktree{}
		if path != "" {
			out = worktrees(r.Context(), path)
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
