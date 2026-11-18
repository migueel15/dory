package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Search finds tasks and memories whose title or text contain every word of
// text, best match first. Archived rows are left out.
func (s *Store) Search(ctx context.Context, text string, limit int) ([]Hit, error) {
	words := strings.Fields(text)
	if len(words) == 0 || limit < 0 || limit > 1000 {
		return nil, ErrInvalid
	}
	if limit == 0 {
		limit = 20
	}
	// Quote each word so FTS5 operators typed by the user are plain text.
	terms := make([]string, len(words))
	for i, w := range words {
		terms[i] = `"` + strings.ReplaceAll(w, `"`, `""`) + `"*`
	}
	match := strings.Join(terms, " ")
	rows, err := s.db.QueryContext(ctx, `
		SELECT 'task', t.id, t.title, bm25(tasks_fts) FROM tasks_fts JOIN tasks t ON t.rowid = tasks_fts.rowid
		WHERE tasks_fts MATCH ? AND t.archived_at IS NULL
		UNION ALL
		SELECT 'memory', m.id, m.title, bm25(memories_fts) FROM memories_fts JOIN memories m ON m.rowid = memories_fts.rowid
		WHERE memories_fts MATCH ? AND m.archived_at IS NULL
		ORDER BY 4 LIMIT ?`, match, match, limit)
	if err != nil {
		return nil, err
	}
	var hits []Hit
	for rows.Next() {
		var h Hit
		if err = rows.Scan(&h.Entity, &h.ID, &h.Title, &h.Rank); err != nil {
			rows.Close()
			return nil, err
		}
		// bm25 is lower for better matches; expose higher-is-better.
		h.Rank = -h.Rank
		hits = append(hits, h)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for i := range hits {
		found, err := s.query(ctx, hits[i].Entity, Filter{Fields: map[string]any{"id": hits[i].ID}, Limit: 1})
		if err != nil || len(found) == 0 {
			return nil, fmt.Errorf("search: %s %s: %w", hits[i].Entity, hits[i].ID, err)
		}
		hits[i].Record = json.RawMessage(found[0])
	}
	return hits, nil
}

// Blockers returns the unfinished tasks that task id depends on. A task is
// finished when it is cancelled or archived; pull request state joins in
// with the GitHub phase.
func (s *Store) Blockers(ctx context.Context, id string) ([]Task, error) {
	deps, err := s.Dependencies().List(ctx, Filter{Fields: map[string]any{"task_id": id}, Limit: 1000})
	if err != nil {
		return nil, err
	}
	var out []Task
	for _, d := range deps {
		t, err := s.Tasks().Get(ctx, d.DependsOnID)
		if err == ErrNotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		if t.Stage != "cancelled" {
			out = append(out, t)
		}
	}
	return out, nil
}
