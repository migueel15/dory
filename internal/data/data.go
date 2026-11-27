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
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/oplog"
	"github.com/migueel15/dory/internal/schema"
)

// ErrNotFound and ErrInvalid are the domain errors of the store; details are
// added with Msgf.
var (
	ErrNotFound = apperr.New(apperr.NotFound, "resource not found")
	ErrInvalid  = apperr.New(apperr.Invalid, "invalid argument")
)

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
func (s *Store) ThreadPulls() ThreadPullStore       { return ThreadPullStore{Resource[ThreadPull]{s, "thread_pull"}} }
func (s *Store) Dependencies() Resource[Dependency] {
	return Resource[Dependency]{s, "task_dependency"}
}
func (s *Store) Plans() Resource[Plan]       { return Resource[Plan]{s, "plan"} }
func (s *Store) Memories() Resource[Memory]  { return Resource[Memory]{s, "memory"} }
func (s *Store) Threads() Resource[Thread]   { return Resource[Thread]{s, "thread"} }
func (s *Store) Notices() Resource[Notice]   { return Resource[Notice]{s, "notice"} }
func (s *Store) Settings() Resource[Setting] { return Resource[Setting]{s, "setting"} }

func (s *Store) Skills() Resource[Skill] { return Resource[Skill]{s, "skill"} }
func (s *Store) RepositoryLocations() Resource[RepositoryLocation] {
	return Resource[RepositoryLocation]{s, "repository_location"}
}

func (s *Store) SkillStates() Resource[SkillState] {
	return Resource[SkillState]{s, "skill_state"}
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
	return r.get(ctx, r.s.db, id, false)
}

func (r Resource[T]) get(ctx context.Context, q querier, id string, archived bool) (T, error) {
	var zero T
	row, err := r.row(ctx, q, id, archived)
	if err != nil {
		return zero, err
	}
	return decode[T](row)
}

func (r Resource[T]) row(ctx context.Context, q querier, id string, archived bool) (json.RawMessage, error) {
	keys := r.keyFields(id)
	if keys == nil {
		return nil, ErrInvalid
	}
	rows, err := queryRows(ctx, q, r.entity, Filter{Fields: keys, Limit: 1, IncludeArchived: archived})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return rows[0], nil
}

func (r Resource[T]) List(ctx context.Context, f Filter) ([]T, error) {
	return r.list(ctx, r.s.db, f, false)
}

func (r Resource[T]) list(ctx context.Context, q querier, f Filter, lookahead bool) ([]T, error) {
	rows, err := queryRowsLimit(ctx, q, r.entity, f, lookahead)
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

// identity returns the op entity id of a row, creating an id when missing.
func (r Resource[T]) identity(row map[string]any, key []string) (string, error) {
	if r.entity == "repository" {
		remote, _ := row["remote_url"].(string)
		if remote == "" {
			return "", ErrInvalid.Msgf("remote_url required")
		}
		row["id"] = oplog.RepositoryID(remote)
	}
	if len(key) == 1 {
		if id, _ := row["id"].(string); id != "" {
			if strings.Contains(id, "/") {
				return "", fmt.Errorf("%w: id cannot contain a slash", ErrInvalid)
			}
			return id, nil
		}
		row["id"] = newID()
		return row["id"].(string), nil
	}
	parts := make([]string, len(key))
	for i, k := range key {
		switch v := row[k].(type) {
		case string:
			parts[i] = v
		case json.Number:
			parts[i] = v.String()
		}
		if parts[i] == "" || strings.Contains(parts[i], "/") {
			return "", ErrInvalid.Msgf("%s required", k)
		}
	}
	return strings.Join(parts, "/"), nil
}

func (r Resource[T]) keyFields(id string) map[string]any {
	spec, _ := schema.Lookup(r.entity)
	parts := strings.Split(id, "/")
	if len(parts) != len(spec.Key) || slices.Contains(parts, "") {
		return nil
	}
	fields := map[string]any{}
	for i, k := range spec.Key {
		fields[k] = parts[i]
	}
	return fields
}

type querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// queryRows returns matching rows as JSON objects.
func queryRows(ctx context.Context, q querier, entity string, f Filter) ([]json.RawMessage, error) {
	return queryRowsLimit(ctx, q, entity, f, false)
}

func queryRowsLimit(ctx context.Context, q querier, entity string, f Filter, lookahead bool) ([]json.RawMessage, error) {
	spec, ok := schema.Lookup(entity)
	if !ok {
		return nil, ErrInvalid
	}
	if f.Offset < 0 || f.Limit < 0 || f.Limit > 1000 {
		return nil, ErrInvalid
	}
	if f.Limit == 0 {
		f.Limit = 100
	}
	if lookahead {
		f.Limit++
	}
	all := append(append([]string{}, spec.Key...), spec.ColumnNames()...)
	from := spec.Table + " AS row"
	columns := make(map[string]string, len(all)+3)
	for _, col := range all {
		columns[col] = "row." + col
	}
	if entity == "thread" {
		from += ` LEFT JOIN thread_state AS state ON state.thread_id = row.id AND state.node_id = row.node_id
			LEFT JOIN peers ON peers.node_id = row.node_id`
		columns["status"] = "COALESCE(state.status, 'idle')"
		columns["last_activity_at"] = "state.last_activity_at"
		columns["state_stale"] = `json(CASE WHEN row.node_id = (SELECT node_id FROM node_state WHERE id = 1)
			OR (` + PeerConnectedSQL + `) THEN 'false' ELSE 'true' END)`
		all = append(all, "status", "last_activity_at", "state_stale")
	}
	pairs := make([]string, 0, len(all))
	for _, col := range all {
		value := columns[col]
		if spec.IsJSON(col) {
			value = "json(" + value + ")"
		}
		pairs = append(pairs, fmt.Sprintf("'%s', %s", col, value))
	}
	where := []string{"1 = 1"}
	args := []any{}
	if !f.IncludeArchived {
		where = append(where, "row.archived_at IS NULL")
	}
	for col, v := range f.Fields {
		if columns[col] == "" || col == "state_stale" {
			return nil, ErrInvalid.Msgf("unknown field %q", col)
		}
		where = append(where, columns[col]+" = ?")
		args = append(args, v)
	}
	query := fmt.Sprintf("SELECT json_object(%s) FROM %s WHERE %s ORDER BY row.created_at, row.%s LIMIT ? OFFSET ?",
		strings.Join(pairs, ", "), from, strings.Join(where, " AND "), strings.Join(spec.Key, ", row."))
	rows, err := q.QueryContext(ctx, query, append(args, f.Limit, f.Offset)...)
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
	if t, ok := any(&v).(*Thread); ok {
		t.Unread = threadUnread(t.LastActivityAt, t.ReadAt)
	}
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

// NewThreadID reserves an identifier before its workspace is prepared.
func NewThreadID() string { return newID() }

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// LocalDB is for a package’s own local tables. All replicated writes go through data/oplog.
func (s *Store) LocalDB() *sql.DB { return s.db }

// ListAll reads every matching row from the requested offset in bounded pages.
// The read transaction keeps the ordering stable across pages.
func (r Resource[T]) ListAll(ctx context.Context, f Filter) ([]T, error) {
	tx, err := r.s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	f.Limit = 1000
	out := []T{}
	for {
		page, err := r.list(ctx, tx, f, false)
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < f.Limit {
			return out, tx.Commit()
		}
		f.Offset += len(page)
	}
}

// ListPage returns the requested page and the next offset, or zero at the end.
func (r Resource[T]) ListPage(ctx context.Context, f Filter) ([]T, int, error) {
	if f.Limit == 0 {
		f.Limit = 100
	}
	rows, err := r.list(ctx, r.s.db, f, true)
	if err != nil || len(rows) <= f.Limit {
		return rows, 0, err
	}
	return rows[:f.Limit], f.Offset + f.Limit, nil
}
