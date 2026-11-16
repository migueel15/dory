package data

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// RepositoryPath returns where this node keeps a repository cloned. The path
// is local: it never replicates.
func (s *Store) RepositoryPath(ctx context.Context, repositoryID string) (string, error) {
	var path string
	err := s.db.QueryRowContext(ctx, `SELECT path FROM repository_paths WHERE repository_id = ?`, repositoryID).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return path, err
}

// RepositoryPaths returns every local path, by repository id.
func (s *Store) RepositoryPaths(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT repository_id, path FROM repository_paths`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, path string
		if err = rows.Scan(&id, &path); err != nil {
			return nil, err
		}
		out[id] = path
	}
	return out, rows.Err()
}

// SetRepositoryPath records the local clone of a repository; an empty path
// forgets it. The path must be a git checkout.
func (s *Store) SetRepositoryPath(ctx context.Context, repositoryID, path string) error {
	if _, err := s.Repositories().Get(ctx, repositoryID); err != nil {
		return err
	}
	if path == "" {
		_, err := s.db.ExecContext(ctx, `DELETE FROM repository_paths WHERE repository_id = ?`, repositoryID)
		return err
	}
	path = filepath.Clean(path)
	if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
		return errors.Join(ErrInvalid, errors.New("not a git checkout: "+path))
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO repository_paths(repository_id, path, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(repository_id) DO UPDATE SET path = excluded.path, updated_at = excluded.updated_at`,
		repositoryID, path, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
