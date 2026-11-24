package oplog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/migueel15/dory/internal/hlc"
	"github.com/migueel15/dory/internal/schema"
)

// BatchSize bounds the number of operations or rows in a page.
const BatchSize = 500

// ErrSnapshotRequired signals that the requested prefix has been compacted.
var ErrSnapshotRequired = errors.New("oplog: snapshot required")

type stamp struct {
	HLC  string `json:"hlc"`
	Node string `json:"node"`
	Seq  int64  `json:"seq"`
}

func (a stamp) less(b stamp) bool {
	if a.HLC != b.HLC {
		return a.HLC < b.HLC
	}
	if a.Node != b.Node {
		return a.Node < b.Node
	}
	return a.Seq < b.Seq
}

type snapshotColumn struct {
	stamp
	Value json.RawMessage `json:"v"`
}

type snapshotVersion struct {
	stamp
	Base string `json:"base"`
	Text string `json:"text"`
}

// RowSnapshot carries the latest compacted columns and merge history of a row.
type RowSnapshot struct {
	Entity  string                    `json:"entity"`
	ID      string                    `json:"entity_id"`
	Columns map[string]snapshotColumn `json:"columns"`
	Merge   []snapshotVersion         `json:"merge"`
	Horizon Vector                    `json:"horizon"`
	HLC     string                    `json:"hlc"`
}

// SnapshotPage contains a page of rows at one applied horizon.
type SnapshotPage struct {
	Horizon Vector        `json:"horizon"`
	Rows    []RowSnapshot `json:"rows"`
}

// SnapshotBatch carries a complete snapshot and the missing operations after it.
type SnapshotBatch struct {
	Horizon Vector        `json:"horizon"`
	Rows    []RowSnapshot `json:"rows"`
	Ops     []Op          `json:"ops"`
}

// Covers reports whether v includes every sequence in other.
func (v Vector) Covers(other Vector) bool {
	for node, seq := range other {
		if v[node] < seq {
			return false
		}
	}
	return true
}

// Include advances v with the sequences known in other.
func (v Vector) Include(other Vector) {
	for node, seq := range other {
		v[node] = max(v[node], seq)
	}
}

func readHorizon(ctx context.Context, tx *sql.Tx) (Vector, error) {
	var raw string
	err := tx.QueryRowContext(ctx, `SELECT compaction_horizon FROM node_state WHERE id = 1`).Scan(&raw)
	v := Vector{}
	if err == nil {
		err = json.Unmarshal([]byte(raw), &v)
	}
	return v, err
}

func saveHorizon(ctx context.Context, tx *sql.Tx, horizon Vector) error {
	raw, err := json.Marshal(horizon)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE node_state SET compaction_horizon = ?,
		op_seq_floor = MAX(op_seq_floor, COALESCE((SELECT value FROM json_each(?) WHERE key = node_state.node_id), 0))
		WHERE id = 1`, string(raw), string(raw))
	return err
}

func readSnapshot(ctx context.Context, tx *sql.Tx, entity, id string) (RowSnapshot, error) {
	s := RowSnapshot{Entity: entity, ID: id}
	err := scanSnapshot(tx.QueryRowContext(ctx, `SELECT columns, merge, horizon, hlc FROM row_snapshots WHERE entity = ? AND entity_id = ?`, entity, id), &s)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return s, err
}

func scanSnapshot(scanner interface{ Scan(...any) error }, s *RowSnapshot, prefix ...any) error {
	var columns, merge, horizon string
	if err := scanner.Scan(append(prefix, &columns, &merge, &horizon, &s.HLC)...); err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(columns), &s.Columns); err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(merge), &s.Merge); err != nil {
		return err
	}
	return json.Unmarshal([]byte(horizon), &s.Horizon)
}

func (s *RowSnapshot) save(ctx context.Context, tx *sql.Tx) error {
	columns, err := json.Marshal(s.Columns)
	if err != nil {
		return err
	}
	var latest []snapshotVersion
	for _, v := range s.Merge {
		if _, err := tx.ExecContext(ctx, `INSERT INTO row_snapshot_versions
			(entity, entity_id, hlc, node_id, seq, base, text) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT DO NOTHING`, s.Entity, s.ID, v.HLC, v.Node, v.Seq, v.Base, v.Text); err != nil {
			return err
		}
		if len(latest) == 0 {
			latest = []snapshotVersion{v}
		} else if latest[0].stamp.less(v.stamp) {
			latest[0] = v
		}
	}
	versions, err := json.Marshal(latest)
	if err != nil {
		return err
	}
	horizon, err := json.Marshal(s.Horizon)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO row_snapshots(entity, entity_id, columns, merge, horizon, hlc)
		VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(entity, entity_id) DO UPDATE SET
		columns = excluded.columns, merge = excluded.merge, horizon = excluded.horizon, hlc = excluded.hlc`,
		s.Entity, s.ID, string(columns), string(versions), string(horizon), s.HLC)
	return err
}

func (s *RowSnapshot) fold(op Op, merge string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(op.Fields, &fields); err != nil {
		return err
	}
	if s.Columns == nil {
		s.Columns = make(map[string]snapshotColumn)
	}
	at := stamp{op.HLC, op.Node, op.Seq}
	for name, raw := range fields {
		if current, ok := s.Columns[name]; !ok || current.stamp.less(at) {
			s.Columns[name] = snapshotColumn{at, raw}
		}
	}
	if raw, ok := fields[merge]; ok && merge != "" && len(raw) > 0 && raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		s.Merge = append(s.Merge, snapshotVersion{at, op.Base, value})
	}
	s.HLC = max(s.HLC, op.HLC)
	return nil
}

// combinedVersions preserves the original Base of every edit. Folding the
// strings early would make the result depend on the order of compaction.
func combinedVersions(a, b []snapshotVersion) []snapshotVersion {
	versions := make(map[stamp]snapshotVersion, len(a)+len(b))
	for _, v := range a {
		versions[v.stamp] = v
	}
	for _, v := range b {
		versions[v.stamp] = v
	}
	out := make([]snapshotVersion, 0, len(versions))
	for _, v := range versions {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].stamp.less(out[j].stamp) })
	return out
}

func (s *RowSnapshot) combine(other RowSnapshot) {
	if s.Columns == nil {
		s.Columns = make(map[string]snapshotColumn)
	}
	for name, col := range other.Columns {
		if current, ok := s.Columns[name]; !ok || current.stamp.less(col.stamp) {
			s.Columns[name] = col
		}
	}
	s.Merge = combinedVersions(s.Merge, other.Merge)
	if s.Horizon == nil {
		s.Horizon = Vector{}
	}
	s.Horizon.Include(other.Horizon)
	s.HLC = max(s.HLC, other.HLC)
}

// SnapshotPage uses one database view for the page and its horizon. Page -1
// reads only the horizon. A caller must discard pages if the horizon changes.
func (l *Log) SnapshotPage(ctx context.Context, page int) (SnapshotPage, error) {
	var out SnapshotPage
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	out.Horizon, err = readHorizon(ctx, tx)
	if err != nil || page < 0 {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT entity, entity_id, columns, merge, horizon, hlc
		FROM row_snapshots ORDER BY entity, entity_id LIMIT ? OFFSET ?`, BatchSize, int64(page)*BatchSize)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var s RowSnapshot
		if err := scanSnapshot(rows, &s, &s.Entity, &s.ID); err != nil {
			return out, err
		}
		out.Rows = append(out.Rows, s)
	}
	if err := rows.Close(); err != nil {
		return out, err
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	for i := range out.Rows {
		s := &out.Rows[i]
		if len(s.Merge) == 0 {
			continue
		}
		s.Merge, err = s.versionsAfter(ctx, tx, "")
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

func (s *RowSnapshot) versionsAfter(ctx context.Context, tx *sql.Tx, base string) ([]snapshotVersion, error) {
	rows, err := tx.QueryContext(ctx, `SELECT hlc, node_id, seq, base, text FROM row_snapshot_versions
		WHERE entity = ? AND entity_id = ? AND hlc >= ? ORDER BY hlc, node_id, seq`, s.Entity, s.ID, base)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var versions []snapshotVersion
	for rows.Next() {
		var v snapshotVersion
		if err := rows.Scan(&v.HLC, &v.Node, &v.Seq, &v.Base, &v.Text); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// ApplySnapshot installs all pages and the missing tail together so foreign
// keys can be checked against the complete state before advancing the vector.
func (l *Log) ApplySnapshot(ctx context.Context, batch SnapshotBatch) error {
	tx, err := l.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	horizon, err := readHorizon(ctx, tx)
	if err != nil {
		return err
	}
	touched := map[[2]string]bool{}
	var latest hlc.Timestamp
	for _, incoming := range batch.Rows {
		if !batch.Horizon.Covers(incoming.Horizon) {
			return fmt.Errorf("oplog: row snapshot exceeds batch horizon")
		}
		at, err := hlc.Parse(incoming.HLC)
		if err != nil {
			return err
		}
		if latest.Less(at) {
			latest = at
		}
		s, err := readSnapshot(ctx, tx, incoming.Entity, incoming.ID)
		if err != nil {
			return err
		}
		s.combine(incoming)
		if err := s.save(ctx, tx); err != nil {
			return err
		}
		touched[[2]string{s.Entity, s.ID}] = true
	}
	raw, err := json.Marshal(batch.Horizon)
	if err != nil {
		return err
	}
	// A received snapshot represents the original operations even if
	// some of those entities are new to this binary. Retain unknown ops.
	known, err := knownEntities()
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM ops AS o WHERE o.entity IN (SELECT value FROM json_each(?))
		AND o.seq <= COALESCE((SELECT value FROM json_each(?) WHERE key = o.node_id), 0)`, known, string(raw)); err != nil {
		return err
	}
	horizon.Include(batch.Horizon)
	if err := saveHorizon(ctx, tx, horizon); err != nil {
		return err
	}
	return l.apply(ctx, tx, batch.Ops, horizon, touched, latest)
}

func (s *RowSnapshot) values(ctx context.Context, tx *sql.Tx, merge string) (map[string]any, error) {
	values := make(map[string]any, len(s.Columns))
	for name, col := range s.Columns {
		decoder := json.NewDecoder(strings.NewReader(string(col.Value)))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		values[name] = value
	}
	if len(s.Merge) == 0 {
		return values, nil
	}
	winner := s.Merge[0]
	for _, v := range s.Merge[1:] {
		if winner.stamp.less(v.stamp) {
			winner = v
		}
	}
	// A newer non-text value wins in the legacy rule only when the row
	// has a single text operation in its entire history.
	if len(s.Merge) == 1 && s.Columns[merge].stamp != winner.stamp {
		var second bool
		err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM row_snapshot_versions
			WHERE entity = ? AND entity_id = ? LIMIT 1 OFFSET 1)`, s.Entity, s.ID).Scan(&second)
		if err != nil || !second {
			return values, err
		}
	}
	archived, err := s.versionsAfter(ctx, tx, winner.Base)
	if err != nil {
		return nil, err
	}
	versions := combinedVersions(s.Merge, archived)
	history := make([]version, 0, len(versions))
	for _, v := range versions {
		history = append(history, version{v.HLC, v.Base, v.Text})
	}
	values[merge] = merged(history)
	return values, nil
}

func knownEntities() (string, error) {
	var names []string
	for _, entity := range schema.All() {
		names = append(names, entity.Name)
	}
	raw, err := json.Marshal(names)
	return string(raw), err
}
