package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/runtime"
)

// mountPulls serves the review aids of a pull request: files marked as
// reviewed on this node and the side chat with an agent (requirement RF-25).
func (s *Server) mountPulls(mux *http.ServeMux) {
	base := "/api/github/repos/{owner}/{repo}/pulls/{number}"
	target := func(w http.ResponseWriter, r *http.Request) (string, int, bool) {
		n, err := strconv.Atoi(r.PathValue("number"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid number")
			return "", 0, false
		}
		return r.PathValue("owner") + "/" + r.PathValue("repo"), n, true
	}
	db := s.Store.DB()
	mux.HandleFunc("GET "+base+"/reviews", func(w http.ResponseWriter, r *http.Request) {
		repo, n, ok := target(w, r)
		if !ok {
			return
		}
		rows, err := db.QueryContext(r.Context(), `SELECT path, patch_hash, reviewed_at FROM pr_file_reviews WHERE repo = ? AND number = ?`, repo, n)
		if err != nil {
			writeDataError(w, err)
			return
		}
		defer rows.Close()
		type mark struct {
			Path       string `json:"path"`
			PatchHash  string `json:"patch_hash"`
			ReviewedAt string `json:"reviewed_at"`
		}
		out := []mark{}
		for rows.Next() {
			var m mark
			if err = rows.Scan(&m.Path, &m.PatchHash, &m.ReviewedAt); err != nil {
				writeDataError(w, err)
				return
			}
			out = append(out, m)
		}
		writeJSON(w, http.StatusOK, out)
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
		_, err := db.ExecContext(r.Context(), `INSERT INTO pr_file_reviews(repo, number, path, patch_hash, reviewed_at) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(repo, number, path) DO UPDATE SET patch_hash = excluded.patch_hash, reviewed_at = excluded.reviewed_at`,
			repo, n, body.Path, body.PatchHash, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			writeDataError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE "+base+"/reviews", func(w http.ResponseWriter, r *http.Request) {
		repo, n, ok := target(w, r)
		if !ok {
			return
		}
		if _, err := db.ExecContext(r.Context(), `DELETE FROM pr_file_reviews WHERE repo = ? AND number = ? AND path = ?`, repo, n, r.URL.Query().Get("path")); err != nil {
			writeDataError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET "+base+"/agent", func(w http.ResponseWriter, r *http.Request) {
		repo, n, ok := target(w, r)
		if !ok {
			return
		}
		t, err := s.pullThread(r.Context(), repo, n)
		if err != nil {
			writeDataError(w, err)
			return
		}
		if t == nil {
			writeJSON(w, http.StatusOK, map[string]any{"available": false})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"available": true, "thread_id": t.ID})
	})
	// ask sends a question about the pull request to its review thread,
	// creating the thread on the first question.
	ask := func(w http.ResponseWriter, r *http.Request) {
		repo, n, ok := target(w, r)
		if !ok {
			return
		}
		var body struct {
			Path     string `json:"path"`
			Start    int    `json:"start"`
			End      int    `json:"end"`
			Snippet  string `json:"snippet"`
			Question string `json:"question"`
			Launch   *struct {
				Provider    string `json:"provider"`
				Model       string `json:"model"`
				Effort      string `json:"effort"`
				RuntimeMode string `json:"runtime_mode"`
			} `json:"launch"`
		}
		if !decode(w, r, &body) {
			return
		}
		ctx := r.Context()
		t, err := s.pullThread(ctx, repo, n)
		if err != nil {
			writeDataError(w, err)
			return
		}
		var first string
		if t == nil {
			if body.Launch == nil {
				writeError(w, http.StatusConflict, "no_thread", "the pull request has no review thread yet: send launch")
				return
			}
			project, path := s.repoLocation(ctx, repo)
			created, err := s.Runtime.Create(ctx, runtime.NewThread{ProjectID: project, Provider: body.Launch.Provider, Model: body.Launch.Model,
				Effort: body.Launch.Effort, RuntimeMode: body.Launch.RuntimeMode, Title: pullTitle(repo, n), RepoPath: path})
			if err != nil {
				writeDataError(w, err)
				return
			}
			t = &created
			first = fmt.Sprintf("Revisas la pull request #%d de %s. Léela con `gh pr view %d --repo %s` y `gh pr diff %d --repo %s`. No modifiques nada salvo que te lo pida. Responde a mis preguntas sobre ella.\n\n", n, repo, n, repo, n, repo)
		}
		var b strings.Builder
		b.WriteString(first)
		if body.Path != "" {
			if body.Start > 0 {
				fmt.Fprintf(&b, "Sobre `%s` líneas %d-%d:\n", body.Path, body.Start, body.End)
			} else {
				fmt.Fprintf(&b, "Sobre `%s`:\n", body.Path)
			}
			if body.Snippet != "" {
				fmt.Fprintf(&b, "```\n%s\n```\n", body.Snippet)
			}
		}
		b.WriteString(body.Question)
		if strings.TrimSpace(b.String()) != "" {
			if err = s.Runtime.Send(ctx, t.ID, b.String()); err != nil {
				writeDataError(w, err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"available": true, "thread_id": t.ID})
	}
	mux.HandleFunc("POST "+base+"/ask", ask)
	mux.HandleFunc("POST "+base+"/agent", ask)
}

func pullTitle(repo string, n int) string { return fmt.Sprintf("Revisión de %s#%d", repo, n) }

// pullThread finds the open review thread of a pull request.
func (s *Server) pullThread(ctx context.Context, repo string, n int) (*data.Thread, error) {
	threads, err := s.Store.Threads().List(ctx, data.Filter{Fields: map[string]any{"title": pullTitle(repo, n)}, Limit: 1})
	if err != nil || len(threads) == 0 {
		return nil, err
	}
	return &threads[0], nil
}

// repoLocation returns the project of a GitHub repository and where this
// node keeps it.
func (s *Server) repoLocation(ctx context.Context, repo string) (*string, string) {
	repos, _ := s.Store.Repositories().List(ctx, data.Filter{Limit: 1000})
	for _, r := range repos {
		if name, ok := github.ParseRemote(r.RemoteURL); ok && strings.EqualFold(name, repo) {
			path, _ := s.Store.RepositoryPath(ctx, r.ID)
			project := r.ProjectID
			return &project, path
		}
	}
	return nil, ""
}
