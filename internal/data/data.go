// Package data is the persistence boundary of a Dory v2 node. Reads come from
// the local SQLite tables; every write goes through the operation log with
// only the fields that changed, so it replicates to the other nodes.
package data

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/oplog"
)

var ErrNotFound = errors.New("resource not found")
var ErrInvalid = errors.New("invalid argument")

// Store gives typed access to the replicated tables.
type Store struct {
	db  *sql.DB
	log *oplog.Log
	now func() time.Time
}

func New(db *sql.DB, log *oplog.Log) *Store {
	return &Store{db: db, log: log, now: time.Now}
}

func (s *Store) Projects() Resource[Project]        { return Resource[Project]{s, "project"} }
func (s *Store) Repositories() Resource[Repository] { return Resource[Repository]{s, "repository"} }
func (s *Store) Tasks() Resource[Task]              { return Resource[Task]{s, "task"} }
func (s *Store) Dependencies() Resource[Dependency] {
	return Resource[Dependency]{s, "task_dependency"}
}
func (s *Store) Plans() Resource[Plan]      { return Resource[Plan]{s, "plan"} }
func (s *Store) Memories() Resource[Memory] { return Resource[Memory]{s, "memory"} }

// jsonColumns are stored as JSON text and read back as JSON values.
var jsonColumns = []string{"sections", "open_questions", "labels"}

// defaults fill columns left empty by a Put.
var defaults = map[string]map[string]any{
	"repository": {"default_branch": "main"},
	"task":       {"stage": "inbox"},
	"plan":       {"author": "agent", "sections": map[string]any{}, "open_questions": []any{}},
	"memory":     {"kind": "context", "labels": []any{}},
}

// Filter matches columns exactly. Limit defaults to 100 (maximum 1000).
type Filter struct {
	Fields          map[string]any
	Limit, Offset   int
	IncludeArchived bool
}

// Resource reads and writes one entity.
type Resource[T any] struct {
	s      *Store
	entity string
}

func (r Resource[T]) Get(ctx context.Context, id string) (T, error) {
	var zero T
	rows, err := r.s.query(ctx, r.entity, Filter{Fields: r.keyFields(id), Limit: 1})
	if err != nil {
		return zero, err
	}
	if len(rows) == 0 {
		return zero, ErrNotFound
	}
	return decode[T](rows[0])
}

func (r Resource[T]) List(ctx context.Context, f Filter) ([]T, error) {
	rows, err := r.s.query(ctx, r.entity, f)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(rows))
	for _, row := range rows {
		v, err := decode[T](row)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// Put creates or replaces a row. Only the fields that differ from the stored
// row become an operation; an unchanged Put writes nothing.
func (r Resource[T]) Put(ctx context.Context, v T) (T, error) {
	var zero T
	raw, err := json.Marshal(v)
	if err != nil {
		return zero, err
	}
	next := map[string]any{}
	if err = json.Unmarshal(raw, &next); err != nil {
		return zero, err
	}
	key, columns, _ := oplog.Columns(r.entity)
	id, err := r.identity(next, key)
	if err != nil {
		return zero, err
	}
	current := map[string]any{}
	if rows, err := r.s.query(ctx, r.entity, Filter{Fields: r.keyFields(id), Limit: 1, IncludeArchived: true}); err != nil {
		return zero, err
	} else if len(rows) > 0 {
		json.Unmarshal(rows[0], &current)
	}
	for col, def := range defaults[r.entity] {
		if empty(next[col]) {
			next[col] = def
		}
	}
	now := r.s.now().UTC().Format(time.RFC3339Nano)
	changed := map[string]any{}
	for _, col := range columns {
		if col == "created_at" || col == "updated_at" {
			continue
		}
		if !same(next[col], current[col]) {
			changed[col] = next[col]
		}
	}
	if len(changed) == 0 && len(current) > 0 {
		return decode[T](mustJSON(current))
	}
	if len(current) == 0 {
		changed["created_at"] = now
	}
	if slices.Contains(columns, "updated_at") {
		changed["updated_at"] = now
	}
	if _, err = r.s.log.Write(ctx, r.entity, id, changed); err != nil {
		return zero, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return r.getAny(ctx, id)
}

// Archive hides a row. It replicates like any other change.
func (r Resource[T]) Archive(ctx context.Context, id string) error {
	if _, err := r.Get(ctx, id); err != nil {
		return err
	}
	now := r.s.now().UTC().Format(time.RFC3339Nano)
	fields := map[string]any{"archived_at": now}
	if _, columns, _ := oplog.Columns(r.entity); slices.Contains(columns, "updated_at") {
		fields["updated_at"] = now
	}
	_, err := r.s.log.Write(ctx, r.entity, id, fields)
	return err
}

func (r Resource[T]) getAny(ctx context.Context, id string) (T, error) {
	var zero T
	rows, err := r.s.query(ctx, r.entity, Filter{Fields: r.keyFields(id), Limit: 1, IncludeArchived: true})
	if err != nil || len(rows) == 0 {
		return zero, errors.Join(ErrNotFound, err)
	}
	return decode[T](rows[0])
}

// identity returns the op entity id of a row, creating an id when missing.
func (r Resource[T]) identity(row map[string]any, key []string) (string, error) {
	if r.entity == "repository" {
		remote, _ := row["remote_url"].(string)
		if remote == "" {
			return "", fmt.Errorf("%w: remote_url required", ErrInvalid)
		}
		row["id"] = oplog.RepositoryID(remote)
	}
	if len(key) == 1 {
		if id, _ := row["id"].(string); id != "" {
			return id, nil
		}
		row["id"] = newID()
		return row["id"].(string), nil
	}
	parts := make([]string, len(key))
	for i, k := range key {
		parts[i], _ = row[k].(string)
		if parts[i] == "" || strings.Contains(parts[i], "/") {
			return "", fmt.Errorf("%w: %s required", ErrInvalid, k)
		}
	}
	return strings.Join(parts, "/"), nil
}

func (r Resource[T]) keyFields(id string) map[string]any {
	key, _, _ := oplog.Columns(r.entity)
	parts := strings.Split(id, "/")
	fields := map[string]any{}
	for i, k := range key {
		if i < len(parts) {
			fields[k] = parts[i]
		}
	}
	return fields
}

// query returns matching rows as JSON objects.
func (s *Store) query(ctx context.Context, entity string, f Filter) ([]json.RawMessage, error) {
	key, columns, ok := oplog.Columns(entity)
	if !ok {
		return nil, ErrInvalid
	}
	if f.Offset < 0 || f.Limit < 0 || f.Limit > 1000 {
		return nil, ErrInvalid
	}
	if f.Limit == 0 {
		f.Limit = 100
	}
	all := append(append([]string{}, key...), columns...)
	pairs := make([]string, 0, len(all))
	for _, col := range all {
		value := col
		if slices.Contains(jsonColumns, col) {
			value = "json(" + col + ")"
		}
		pairs = append(pairs, fmt.Sprintf("'%s', %s", col, value))
	}
	where := []string{"1 = 1"}
	args := []any{}
	if !f.IncludeArchived {
		where = append(where, "archived_at IS NULL")
	}
	for col, v := range f.Fields {
		if !slices.Contains(all, col) {
			return nil, fmt.Errorf("%w: unknown field %q", ErrInvalid, col)
		}
		where = append(where, col+" = ?")
		args = append(args, v)
	}
	table := map[string]string{"project": "projects", "repository": "repositories", "task": "tasks",
		"task_dependency": "task_dependencies", "plan": "plans", "memory": "memories"}[entity]
	query := fmt.Sprintf("SELECT json_object(%s) FROM %s WHERE %s ORDER BY created_at, %s LIMIT ? OFFSET ?",
		strings.Join(pairs, ", "), table, strings.Join(where, " AND "), strings.Join(key, ", "))
	rows, err := s.db.QueryContext(ctx, query, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(raw))
	}
	return out, rows.Err()
}

func decode[T any](raw []byte) (T, error) {
	var v T
	err := json.Unmarshal(raw, &v)
	return v, err
}

// same compares two decoded JSON values by their canonical encoding.
func same(a, b any) bool { return bytes.Equal(mustJSON(a), mustJSON(b)) }

func empty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return false
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
