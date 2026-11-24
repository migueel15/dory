package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/workspace"
)

// Folder is a directory of a node, as the project picker lists it.
type Folder struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Git  bool   `json:"git"`
}

// Checkout is what Dory detects in a folder to create a project from it.
type Checkout struct {
	Path          string `json:"path"`
	Name          string `json:"name"`
	Git           bool   `json:"git"`
	Remote        string `json:"remote"`
	DefaultBranch string `json:"default_branch"`
}

// mountFolders lets the interface browse the folders of any node to pick
// a project, and record where that node keeps a repository.
func (s *Server) mountFolders(mux *http.ServeMux) {
	here := s.onNode
	mux.HandleFunc("GET /api/nodes/{node}/folders", here(func(w http.ResponseWriter, r *http.Request) {
		dir := r.URL.Query().Get("path")
		if dir == "" {
			dir, _ = os.UserHomeDir()
		}
		dir = filepath.Clean(dir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
			return
		}
		folders := []Folder{}
		showHidden := r.URL.Query().Get("hidden") == "true"
		for _, e := range entries {
			if !e.IsDir() || !showHidden && strings.HasPrefix(e.Name(), ".") {
				continue
			}
			p := filepath.Join(dir, e.Name())
			_, err := os.Stat(filepath.Join(p, ".git"))
			folders = append(folders, Folder{Name: e.Name(), Path: p, Git: err == nil})
		}
		sort.Slice(folders, func(i, j int) bool { return strings.ToLower(folders[i].Name) < strings.ToLower(folders[j].Name) })
		home, _ := os.UserHomeDir()
		parent := filepath.Dir(dir)
		if parent == dir {
			parent = ""
		}
		writeJSON(w, http.StatusOK, map[string]any{"path": dir, "parent": parent, "home": home, "folders": folders})
	}))
	mux.HandleFunc("GET /api/nodes/{node}/checkout", here(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, inspect(r.Context(), filepath.Clean(r.URL.Query().Get("path"))))
	}))
	mux.HandleFunc("PUT /api/nodes/{node}/repositories/{id}/path", here(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Path string `json:"path"`
		}
		if !decode(w, r, &body) {
			return
		}
		// The repository row may still be on its way from the node that created it.
		var err error
		for range 20 {
			if err = s.Store.SetRepositoryPath(r.Context(), r.PathValue("id"), body.Path); err == nil {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		if err != nil {
			writeDataError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

// inspect reads the name, remote and default branch of a checkout.
func inspect(ctx context.Context, dir string) Checkout {
	c := Checkout{Path: dir, Name: filepath.Base(dir)}
	git := func(args ...string) string {
		out, err := workspace.Output(ctx, dir, args...)
		if err != nil {
			return ""
		}
		return out
	}
	if git("rev-parse", "--is-inside-work-tree") != "true" {
		return c
	}
	c.Git = true
	if top := git("rev-parse", "--show-toplevel"); top != "" {
		c.Path, c.Name = top, filepath.Base(top)
	}
	c.Remote = git("remote", "get-url", "origin")
	if ref := git("symbolic-ref", "--short", "refs/remotes/origin/HEAD"); ref != "" {
		c.DefaultBranch = strings.TrimPrefix(ref, "origin/")
	} else {
		c.DefaultBranch = git("branch", "--show-current")
	}
	if name, ok := strings.CutSuffix(filepath.Base(strings.TrimSuffix(c.Remote, "/")), ".git"); ok || name != "" && c.Remote != "" {
		c.Name = name
	}
	return c
}

// onNode runs h here when the path names this node, or forwards it to the
// node it names.
func (s *Server) onNode(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		node := r.PathValue("node")
		if node == "local" || node == s.Node.ID {
			h(w, r)
			return
		}
		if s.Remote == nil || !s.Remote.Forward(w, r, node) {
			writeError(w, http.StatusServiceUnavailable, "node_offline", "el nodo no está conectado")
		}
	}
}
