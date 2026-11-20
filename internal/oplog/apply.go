package oplog

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/hlc"
)

// table describes a replicated table. Key columns identify the row; for a
// single id column the op entity id is the row id, for composite keys the
// entity id is the key values joined by "/".
type table struct {
	name    string
	key     []string
	columns []string
	// unique is a column that must not repeat. When two nodes pick the same
	// value offline, the row with the smallest id keeps it and the others get
	// a suffix, the same on every node.
	unique string
	// merge is a text column where concurrent edits keep every version: the
	// winner first, then each text it did not see, so no text is lost.
	merge string
}

// EnsureThreadTask creates a task and links it to a thread in the same local
// transaction. Repeated calls reuse the link, including after a lost response.
func (l *Log) EnsureThreadTask(ctx context.Context, thread, title, description string) (string, error) {
	tx, err := l.begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var existing, project sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT task_id, project_id FROM threads WHERE id = ? AND archived_at IS NULL`, thread).Scan(&existing, &project); err != nil {
		return "", err
	}
	if existing.Valid && existing.String != "" {
		return existing.String, nil
	}
	if strings.TrimSpace(title) == "" {
		return "", fmt.Errorf("title is required")
	}
	sum := sha256.Sum256([]byte("dory:plan-task:" + thread))
	sum[6] = (sum[6] & 0x0f) | 0x50
	sum[8] = (sum[8] & 0x3f) | 0x80
	id := fmt.Sprintf("%x-%x-%x-%x-%x", sum[:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM tasks WHERE id = ?`, id).Scan(&exists); err != nil {
		return "", err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if exists == 0 {
		fields := map[string]any{"title": title, "description": description, "stage": "inbox", "created_at": now, "updated_at": now}
		if project.Valid {
			fields["project_id"] = project.String
		}
		if _, err = l.Append(ctx, tx, "task", id, fields); err != nil {
			return "", err
		}
		if err = materialize(ctx, tx, "task", id); err != nil {
			return "", err
		}
	} else {
		var archived sql.NullString
		if err = tx.QueryRowContext(ctx, `SELECT archived_at FROM tasks WHERE id = ?`, id).Scan(&archived); err != nil {
			return "", err
		}
		if archived.Valid {
			return "", fmt.Errorf("the task previously linked to this thread is archived")
		}
	}
	if _, err = l.Append(ctx, tx, "thread", thread, map[string]any{"task_id": id, "updated_at": now}); err != nil {
		return "", err
	}
	if err = materialize(ctx, tx, "thread", thread); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	if l.OnChange != nil {
		if exists == 0 {
			l.OnChange("task", id)
		}
		l.OnChange("thread", thread)
	}
	return id, nil
}

var tables = map[string]table{
	"project":             {"projects", []string{"id"}, []string{"slug", "name", "description", "created_at", "updated_at", "archived_at"}, "slug", ""},
	"repository":          {"repositories", []string{"id"}, []string{"project_id", "remote_url", "default_branch", "created_at", "updated_at", "archived_at"}, "", ""},
	"repository_location": {"repository_locations", []string{"repository_id", "node_id"}, []string{"path", "created_at", "updated_at", "archived_at"}, "", ""},
	"task":                {"tasks", []string{"id"}, []string{"project_id", "repository_id", "title", "description", "stage", "branch", "issue_number", "pr_number", "position", "created_at", "updated_at", "archived_at"}, "", ""},
	"task_dependency":     {"task_dependencies", []string{"task_id", "depends_on_id"}, []string{"created_at", "archived_at"}, "", ""},
	"plan":                {"plans", []string{"id"}, []string{"task_id", "version", "sections", "open_questions", "author", "accepted_at", "body_md", "revision", "created_at", "updated_at", "archived_at"}, "", ""},
	"memory":              {"memories", []string{"id"}, []string{"scope", "scope_id", "kind", "title", "content", "labels", "created_at", "updated_at", "archived_at"}, "", "content"},
	"skill":               {"skills", []string{"id"}, []string{"name", "description", "body_md", "files", "version", "targets", "created_at", "updated_at", "archived_at"}, "name", ""},
	"skill_state":         {"skill_states", []string{"skill_id", "node_id"}, []string{"status", "version", "message", "created_at", "updated_at", "archived_at"}, "", ""},
	"thread":              {"threads", []string{"id"}, []string{"node_id", "project_id", "task_id", "task_linked_at", "parent_id", "provider", "model", "effort", "runtime_mode", "title", "branch", "status", "pinned_at", "read_at", "last_activity_at", "snoozed_until", "settled_at", "created_at", "updated_at", "archived_at"}, "", ""},
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
	if remote, ok := fields["remote_url"].(string); ok && entity == "repository" && id != RepositoryID(remote) {
		return Op{}, fmt.Errorf("oplog: repository id must be RepositoryID(%q)", remote)
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
	if err == nil && l.OnChange != nil {
		l.OnChange(entity, id)
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
		res, err := tx.ExecContext(ctx, `INSERT INTO ops(node_id, seq, hlc, entity, entity_id, fields, base) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
			op.Node, op.Seq, op.HLC, op.Entity, op.EntityID, string(op.Fields), op.Base)
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
	if err = tx.Commit(); err != nil {
		return err
	}
	if l.OnChange != nil {
		for key := range touched {
			l.OnChange(key[0], key[1])
		}
	}
	return nil
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
	rows, err := tx.QueryContext(ctx, `SELECT fields, hlc, base FROM ops WHERE entity = ? AND entity_id = ? ORDER BY hlc`, entity, id)
	if err != nil {
		return err
	}
	values := map[string]any{}
	var versions []version
	for rows.Next() {
		var raw string
		var v version
		if err = rows.Scan(&raw, &v.hlc, &v.base); err != nil {
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
		if text, ok := fields[t.merge].(string); ok && t.merge != "" {
			v.text = text
			versions = append(versions, v)
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	if len(versions) > 1 {
		values[t.merge] = merged(versions)
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
		if name == t.unique {
			if v, err = claim(ctx, tx, entity, id, v); err != nil {
				return err
			}
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

// claim resolves the unique column of a row. The smallest id among the rows
// whose latest wish is value keeps it; the rest get value plus a suffix of
// their own id. If this row owns value, rows already holding it are renamed.
func claim(ctx context.Context, tx *sql.Tx, entity, id string, value any) (any, error) {
	t := tables[entity]
	path := "$." + t.unique
	var owner string
	err := tx.QueryRowContext(ctx, `SELECT MIN(o.entity_id) FROM ops o
		WHERE o.entity = ? AND json_extract(o.fields, ?) = ?
		AND NOT EXISTS (SELECT 1 FROM ops p WHERE p.entity = o.entity AND p.entity_id = o.entity_id
			AND p.hlc > o.hlc AND json_extract(p.fields, ?) IS NOT NULL)`,
		entity, path, value, path).Scan(&owner)
	if err != nil {
		return nil, err
	}
	if owner != id {
		return suffixed(value, id), nil
	}
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`SELECT id FROM %s WHERE %s = ? AND id <> ?`, t.name, t.unique), value, id)
	if err != nil {
		return nil, err
	}
	var holders []string
	for rows.Next() {
		var other string
		if err = rows.Scan(&other); err != nil {
			rows.Close()
			return nil, err
		}
		holders = append(holders, other)
	}
	rows.Close()
	for _, other := range holders {
		if _, err = tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET %s = ? WHERE id = ?`, t.name, t.unique), suffixed(value, other), other); err != nil {
			return nil, err
		}
	}
	return value, nil
}

func suffixed(value any, id string) string {
	return fmt.Sprintf("%v-%s", value, id[:min(8, len(id))])
}

// RepositoryID derives the id of a repository from its remote, so two nodes
// that add the same repository offline create the same row.
func RepositoryID(remote string) string { return remoteID("dory-repository:", remote) }

// ProjectID derives the id of a project created from a repository, so two
// environments that add the same repository create the same project.
func ProjectID(remote string) string { return remoteID("dory-project:", remote) }

func remoteID(namespace, remote string) string {
	u := strings.TrimSpace(remote)
	if rest, ok := strings.CutPrefix(u, "git@"); ok {
		u = "https://" + strings.Replace(rest, ":", "/", 1)
	}
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	u = strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(u, "/"), ".git"))
	sum := sha1.Sum([]byte(namespace + u))
	sum[6] = sum[6]&0x0f | 0x50 // version 5
	sum[8] = sum[8]&0x3f | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

type version struct{ hlc, base, text string }

// merged returns the latest text followed by the texts written concurrently
// with it, oldest first. A later edit made after seeing them replaces them.
func merged(versions []version) string {
	winner := versions[len(versions)-1]
	out := winner.text
	seen := map[string]bool{winner.text: true}
	for _, v := range versions[:len(versions)-1] {
		if v.hlc > winner.base && !seen[v.text] {
			seen[v.text] = true
			out += "\n\n---\nVersión en conflicto:\n\n" + v.text
		}
	}
	return out
}

// Columns returns the key and the replicated columns of an entity.
func Columns(entity string) (key, columns []string, ok bool) {
	t, ok := tables[entity]
	return t.key, t.columns, ok
}
