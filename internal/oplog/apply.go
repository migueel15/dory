package oplog

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/migueel15/dory/internal/hlc"
)

// table describes a replicated table. Key columns identify the row; for a
// single id column the op entity id is the row id, for composite keys the
// entity id is the key values joined by "/".
type table struct {
	name    string
	key     []string
	columns []string
}

var tables = map[string]table{
	"project":         {"projects", []string{"id"}, []string{"slug", "name", "description", "created_at", "updated_at", "archived_at"}},
	"repository":      {"repositories", []string{"id"}, []string{"project_id", "remote_url", "default_branch", "created_at", "updated_at", "archived_at"}},
	"task":            {"tasks", []string{"id"}, []string{"project_id", "repository_id", "title", "description", "stage", "branch", "issue_number", "pr_number", "position", "created_at", "updated_at", "archived_at"}},
	"task_dependency": {"task_dependencies", []string{"task_id", "depends_on_id"}, []string{"created_at", "archived_at"}},
	"plan":            {"plans", []string{"id"}, []string{"task_id", "version", "sections", "open_questions", "author", "accepted_at", "created_at", "updated_at", "archived_at"}},
	"memory":          {"memories", []string{"id"}, []string{"scope", "scope_id", "kind", "title", "content", "labels", "created_at", "updated_at", "archived_at"}},
}

// Write records a local change and applies it to its table in one
// transaction. Unknown entities or columns are refused.
func (l *Log) Write(ctx context.Context, entity, id string, fields map[string]any) (Op, error) {
	t, ok := tables[entity]
	if !ok {
		return Op{}, fmt.Errorf("oplog: unknown entity %q", entity)
	}
	for name := range fields {
		if !slices.Contains(t.columns, name) {
			return Op{}, fmt.Errorf("oplog: %s has no column %q", entity, name)
		}
	}
	tx, err := l.begin(ctx)
	if err != nil {
		return Op{}, err
	}
	defer tx.Rollback()
	op, err := l.Append(ctx, tx, entity, id, fields)
	if err == nil {
		err = materialize(ctx, tx, entity, id)
	}
	if err == nil {
		err = tx.Commit()
	}
	return op, err
}

// Apply stores operations received from another node and updates the rows
// they touch. Operations already known are skipped, so applying twice is safe.
func (l *Log) Apply(ctx context.Context, ops []Op) error {
	tx, err := l.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	touched := map[[2]string]bool{}
	var latest hlc.Timestamp
	for _, op := range ops {
		if _, ok := tables[op.Entity]; !ok {
			return fmt.Errorf("oplog: unknown entity %q", op.Entity)
		}
		ts, err := hlc.Parse(op.HLC)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO ops(node_id, seq, hlc, entity, entity_id, fields) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
			op.Node, op.Seq, op.HLC, op.Entity, op.EntityID, string(op.Fields))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue
		}
		touched[[2]string{op.Entity, op.EntityID}] = true
		if latest.Less(ts) {
			latest = ts
		}
	}
	for key := range touched {
		if err = materialize(ctx, tx, key[0], key[1]); err != nil {
			return err
		}
	}
	if len(touched) > 0 {
		next := l.clock.Observe(latest)
		if _, err = tx.ExecContext(ctx, `UPDATE node_state SET hlc = ? WHERE id = 1`, next.String()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// begin defers foreign key checks to commit, because ops from several nodes
// may reach a child row before its parent within one batch.
func (l *Log) begin(ctx context.Context) (*sql.Tx, error) {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}

// materialize rebuilds a row from all its operations: each column takes the
// value of the latest operation, by clock, that set it.
func materialize(ctx context.Context, tx *sql.Tx, entity, id string) error {
	t := tables[entity]
	rows, err := tx.QueryContext(ctx, `SELECT fields FROM ops WHERE entity = ? AND entity_id = ? ORDER BY hlc`, entity, id)
	if err != nil {
		return err
	}
	values := map[string]any{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		fields := map[string]any{}
		dec := json.NewDecoder(strings.NewReader(raw))
		dec.UseNumber()
		if err = dec.Decode(&fields); err != nil {
			rows.Close()
			return err
		}
		for name, v := range fields {
			values[name] = v
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}

	keys := strings.Split(id, "/")
	if len(keys) != len(t.key) {
		return fmt.Errorf("oplog: %s id %q needs %d parts", entity, id, len(t.key))
	}
	cols := append([]string{}, t.key...)
	args := make([]any, 0, len(t.key)+len(values))
	for _, k := range keys {
		args = append(args, k)
	}
	set := []string{}
	names := make([]string, 0, len(values))
	for name := range values {
		if slices.Contains(t.columns, name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		v, err := column(values[name])
		if err != nil {
			return err
		}
		cols = append(cols, name)
		args = append(args, v)
		set = append(set, name+" = excluded."+name)
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO ",
		t.name, strings.Join(cols, ", "), strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", "), strings.Join(t.key, ", "))
	if len(set) == 0 {
		query += "NOTHING"
	} else {
		query += "UPDATE SET " + strings.Join(set, ", ")
	}
	_, err = tx.ExecContext(ctx, query, args...)
	return err
}

// column converts a decoded JSON value to what SQLite stores: objects and
// arrays as JSON text, numbers as integers when they have no fraction.
func column(v any) (any, error) {
	switch x := v.(type) {
	case map[string]any, []any:
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(x); err != nil {
			return nil, err
		}
		return strings.TrimSuffix(buf.String(), "\n"), nil
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return n, nil
		}
		return x.Float64()
	default:
		return x, nil
	}
}
