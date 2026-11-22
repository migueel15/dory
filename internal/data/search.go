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
	return s.search(ctx, text, limit, false)
}

// SearchTasks limits the index to tasks before applying the result limit.
func (s *Store) SearchTasks(ctx context.Context, text string, limit int) ([]Hit, error) {
	return s.search(ctx, text, limit, true)
}

// MemoryScope narrows SearchMemories before its limit applies, so a match of
// the wanted project is not pushed out by matches of other projects.
type MemoryScope struct {
	Scope   string
	ScopeID string
	Kind    string
}

// SearchMemories finds memories that contain every word of text, best match
// first, filtered by scope and kind in the query itself.
func (s *Store) SearchMemories(ctx context.Context, text string, in MemoryScope, limit int) ([]Hit, error) {
	match, ok := ftsMatch(text)
	if !ok || limit <= 0 || limit > 1000 {
		return nil, ErrInvalid
	}
	query := `SELECT 'memory', m.id, m.title, bm25(memories_fts) FROM memories_fts JOIN memories m ON m.rowid = memories_fts.rowid
		WHERE memories_fts MATCH ? AND m.archived_at IS NULL`
	args := []any{match}
	for col, v := range map[string]string{"m.scope": in.Scope, "m.scope_id": in.ScopeID, "m.kind": in.Kind} {
		if v != "" {
			query += " AND " + col + " = ?"
			args = append(args, v)
		}
	}
	return s.runSearch(ctx, query+" ORDER BY 4 LIMIT ?", append(args, limit))
}

// ftsMatch quotes each word so FTS5 operators typed by the user are plain text.
func ftsMatch(text string) (string, bool) {
	words := strings.Fields(text)
	if len(words) == 0 {
		return "", false
	}
	terms := make([]string, len(words))
	for i, w := range words {
		terms[i] = `"` + strings.ReplaceAll(w, `"`, `""`) + `"*`
	}
	return strings.Join(terms, " "), true
}

func (s *Store) search(ctx context.Context, text string, limit int, tasksOnly bool) ([]Hit, error) {
	match, ok := ftsMatch(text)
	if !ok || limit < 0 || limit > 1000 {
		return nil, ErrInvalid
	}
	if limit == 0 {
		limit = 20
	}
	query := `
		SELECT 'task', t.id, t.title, bm25(tasks_fts) FROM tasks_fts JOIN tasks t ON t.rowid = tasks_fts.rowid
		WHERE tasks_fts MATCH ? AND t.archived_at IS NULL
		UNION ALL
		SELECT 'memory', m.id, m.title, bm25(memories_fts) FROM memories_fts JOIN memories m ON m.rowid = memories_fts.rowid
		WHERE memories_fts MATCH ? AND m.archived_at IS NULL
		ORDER BY 4 LIMIT ?`
	args := []any{match, match, limit}
	if tasksOnly {
		query = `SELECT 'task', t.id, t.title, bm25(tasks_fts) FROM tasks_fts JOIN tasks t ON t.rowid = tasks_fts.rowid WHERE tasks_fts MATCH ? AND t.archived_at IS NULL ORDER BY 4 LIMIT ?`
		args = []any{match, limit}
	}
	return s.runSearch(ctx, query, args)
}

func (s *Store) runSearch(ctx context.Context, query string, args []any) ([]Hit, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
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
