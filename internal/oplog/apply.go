package oplog

import (
	"bytes"
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	stdsync "sync"

	"github.com/migueel15/dory/internal/hlc"
	"github.com/migueel15/dory/internal/schema"
)

var reportedUnknownEntities stdsync.Map

// Apply stores operations received from another node and updates the rows
// they touch. Operations for future entities are kept for replication and
// migration without materializing a row. Applying twice is safe.
func (l *Log) Apply(ctx context.Context, ops []Op) error {
	tx, err := l.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	horizon, err := readHorizon(ctx, tx)
	if err != nil {
		return err
	}
	return l.apply(ctx, tx, ops, horizon, map[[2]string]bool{}, hlc.Timestamp{})
}

func (l *Log) apply(ctx context.Context, tx *sql.Tx, ops []Op, horizon Vector, touched map[[2]string]bool, latest hlc.Timestamp) error {
	for _, op := range ops {
		if op.Seq <= horizon[op.Node] {
			continue
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
		if _, known := schema.Lookup(key[0]); !known {
			continue
		}
		if err := materialize(ctx, tx, key[0], key[1]); err != nil {
			return err
		}
	}
	if len(touched) > 0 {
		next := l.clock.Observe(latest)
		if _, err := tx.ExecContext(ctx, `UPDATE node_state SET hlc = ? WHERE id = 1`, next.String()); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for key := range touched {
		if _, known := schema.Lookup(key[0]); !known {
			if _, reported := reportedUnknownEntities.LoadOrStore(key[0], struct{}{}); !reported {
				log.Printf("oplog: preserved operations for unknown entity %q", key[0])
			}
		}
		if l.OnChange != nil {
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

// materialize rebuilds a row from its snapshot and remaining operations.
// Each column takes the value of the latest operation that set it.
func materialize(ctx context.Context, tx *sql.Tx, entity, id string) error {
	t, _ := schema.Lookup(entity)
	s, err := readSnapshot(ctx, tx, entity, id)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT node_id, seq, hlc, entity, entity_id, fields, base FROM ops
		WHERE entity = ? AND entity_id = ?`, entity, id)
	if err != nil {
		return err
	}
	ops, err := scanOps(rows)
	if err != nil {
		return err
	}
	for _, op := range ops {
		if err := s.fold(op, t.Merge); err != nil {
			return err
		}
	}
	values, err := s.values(ctx, tx, t.Merge)
	if err != nil {
		return err
	}

	keys := strings.Split(id, "/")
	if len(keys) != len(t.Key) {
		return fmt.Errorf("oplog: %s id %q needs %d parts", entity, id, len(t.Key))
	}
	cols := append([]string{}, t.Key...)
	args := make([]any, 0, len(t.Key)+len(values))
	for _, k := range keys {
		args = append(args, k)
	}
	set := []string{}
	names := make([]string, 0, len(values))
	for name := range values {
		if _, ok := t.Column(name); ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var previous string
	var unique any
	for _, name := range names {
		v, err := column(values[name])
		if err != nil {
			return err
		}
		if name == t.Unique {
			err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT %s FROM %s WHERE id = ?`, t.Unique, t.Table), id).Scan(&previous)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if v, err = claim(ctx, tx, entity, id, v); err != nil {
				return err
			}
			unique = v
		}
		cols = append(cols, name)
		args = append(args, v)
		set = append(set, name+" = excluded."+name)
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO ",
		t.Table, strings.Join(cols, ", "), strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", "), strings.Join(t.Key, ", "))
	if len(set) == 0 {
		query += "NOTHING"
	} else {
		query += "UPDATE SET " + strings.Join(set, ", ")
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return err
	}
	if previous != "" && previous != unique {
		owner, err := uniqueOwner(ctx, tx, entity, t.Unique, previous)
		if err != nil {
			return err
		}
		if owner.Valid {
			_, err = tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET %s = ? WHERE id = ?`, t.Table, t.Unique), previous, owner.String)
			return err
		}
	}
	return nil
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
	t, _ := schema.Lookup(entity)
	owner, err := uniqueOwner(ctx, tx, entity, t.Unique, value)
	if err != nil {
		return nil, err
	}
	if !owner.Valid || owner.String != id {
		return suffixed(value, id), nil
	}
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`SELECT id FROM %s WHERE %s = ? AND id <> ?`, t.Table, t.Unique), value, id)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, other := range holders {
		if _, err = tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET %s = ? WHERE id = ?`, t.Table, t.Unique), suffixed(value, other), other); err != nil {
			return nil, err
		}
	}
	return value, nil
}

// uniqueOwner arbitrates over the latest wish of each row, including
// wishes whose original operations have been compacted.
func uniqueOwner(ctx context.Context, tx *sql.Tx, entity, name string, value any) (sql.NullString, error) {
	var owner sql.NullString
	err := tx.QueryRowContext(ctx, `WITH wishes AS (
		SELECT entity_id, hlc, node_id, seq, json_extract(fields, :field) AS value
		FROM ops WHERE entity = :entity AND json_type(fields, :field) IS NOT NULL
		UNION ALL
		SELECT entity_id, json_extract(columns, :hlc), json_extract(columns, :node),
			json_extract(columns, :seq), json_extract(columns, :value)
		FROM row_snapshots WHERE entity = :entity AND json_type(columns, :value) IS NOT NULL
	), ranked AS (
		SELECT entity_id, value, ROW_NUMBER() OVER (PARTITION BY entity_id
			ORDER BY hlc DESC, node_id DESC, seq DESC) AS rank FROM wishes
	)
	SELECT MIN(entity_id) FROM ranked WHERE rank = 1 AND value = :wanted`,
		sql.Named("entity", entity), sql.Named("field", "$."+name), sql.Named("hlc", "$."+name+".hlc"),
		sql.Named("node", "$."+name+".node"), sql.Named("seq", "$."+name+".seq"),
		sql.Named("value", "$."+name+".v"), sql.Named("wanted", value)).Scan(&owner)
	return owner, err
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
