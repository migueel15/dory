// Package reviews keeps which files of a pull request the user reviewed.
package reviews

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Review marks one file as reviewed at the patch with PatchHash.
type Review struct {
	Path       string    `json:"path"`
	PatchHash  string    `json:"patch_hash"`
	ReviewedAt time.Time `json:"reviewed_at"`
}

// Store keeps the marks in the pr_file_reviews table.
type Store struct{ Pool *pgxpool.Pool }

// List returns the marks of a pull request.
func (s Store) List(ctx context.Context, repo string, number int) ([]Review, error) {
	rows, err := s.Pool.Query(ctx, "SELECT path, patch_hash, reviewed_at FROM pr_file_reviews WHERE repo = $1 AND number = $2 ORDER BY path", repo, number)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Review{}
	for rows.Next() {
		var v Review
		if err := rows.Scan(&v.Path, &v.PatchHash, &v.ReviewedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Mark records a file as reviewed, replacing an older mark.
func (s Store) Mark(ctx context.Context, repo string, number int, path, hash string) error {
	_, err := s.Pool.Exec(ctx, "INSERT INTO pr_file_reviews (repo, number, path, patch_hash) VALUES ($1, $2, $3, $4) ON CONFLICT (repo, number, path) DO UPDATE SET patch_hash = excluded.patch_hash, reviewed_at = now()", repo, number, path, hash)
	return err
}

// Unmark removes the mark of a file.
func (s Store) Unmark(ctx context.Context, repo string, number int, path string) error {
	_, err := s.Pool.Exec(ctx, "DELETE FROM pr_file_reviews WHERE repo = $1 AND number = $2 AND path = $3", repo, number, path)
	return err
}

const route = "/api/github/repos/{owner}/{repo}/pulls/{number}/reviews"

// Mount serves GET, PUT and DELETE on the reviews of a pull request. PUT takes
// {path, patch_hash}; DELETE takes ?path=.
func (s Store) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET "+route, func(w http.ResponseWriter, r *http.Request) {
		repo, number, ok := target(w, r)
		if !ok {
			return
		}
		out, err := s.List(r.Context(), repo, number)
		if err != nil {
			http.Error(w, "reviews unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("PUT "+route, func(w http.ResponseWriter, r *http.Request) {
		repo, number, ok := target(w, r)
		if !ok {
			return
		}
		var in Review
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&in) != nil || in.Path == "" || in.PatchHash == "" {
			http.Error(w, "path and patch_hash are required", http.StatusBadRequest)
			return
		}
		if err := s.Mark(r.Context(), repo, number, in.Path, in.PatchHash); err != nil {
			http.Error(w, "reviews unavailable", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE "+route, func(w http.ResponseWriter, r *http.Request) {
		repo, number, ok := target(w, r)
		if !ok {
			return
		}
		path := r.URL.Query().Get("path")
		if path == "" {
			http.Error(w, "path is required", http.StatusBadRequest)
			return
		}
		if err := s.Unmark(r.Context(), repo, number, path); err != nil {
			http.Error(w, "reviews unavailable", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func target(w http.ResponseWriter, r *http.Request) (string, int, bool) {
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil || number <= 0 {
		http.Error(w, "invalid number", http.StatusBadRequest)
		return "", 0, false
	}
	return r.PathValue("owner") + "/" + r.PathValue("repo"), number, true
}
