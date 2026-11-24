// Package oplog keeps the operation log of a node: every change to a
// replicated row, stamped with a hybrid logical clock, so nodes can exchange
// the operations the other one is missing.
package oplog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/migueel15/dory/internal/hlc"
)

// Op is one change. Fields holds only the columns that changed.
type Op struct {
	Node     string          `json:"node"`
	Seq      int64           `json:"seq"`
	HLC      string          `json:"hlc"`
	Entity   string          `json:"entity"`
	EntityID string          `json:"entity_id"`
	Fields   json.RawMessage `json:"fields"`
	// Base is the latest clock of the row that the origin node knew when it
	// wrote. An op whose clock is above the base of another was not seen by it.
	Base string `json:"base,omitempty"`
}

// Vector holds, per origin node, the highest sequence number known locally.
type Vector map[string]int64

// Log appends local operations and reads them back.
type Log struct {
	db    *sql.DB
	node  string
	clock *hlc.Clock
	// OnChange is called after a write, local or received, commits.
	OnChange func(entity, id string)
}

// Open resumes the clock of this node from its stored state.
func Open(ctx context.Context, db *sql.DB) (*Log, error) {
	var node, last string
	if err := db.QueryRowContext(ctx, `SELECT node_id, hlc FROM node_state WHERE id = 1`).Scan(&node, &last); err != nil {
		return nil, fmt.Errorf("oplog: node state: %w", err)
	}
	ts, err := hlc.Parse(last)
	if err != nil {
		return nil, err
	}
	return &Log{db: db, node: node, clock: hlc.New(node, ts)}, nil
}

// Node returns the identity of this node.
func (l *Log) Node() string { return l.node }

// Append records a local change inside tx, the transaction that writes the
// row, so the row and its operation commit together.
func (l *Log) Append(ctx context.Context, tx *sql.Tx, entity, id string, fields map[string]any) (Op, error) {
	raw, err := json.Marshal(fields)
	if err != nil {
		return Op{}, err
	}
	op := Op{Node: l.node, HLC: l.clock.Now().String(), Entity: entity, EntityID: id, Fields: raw}
	err = tx.QueryRowContext(ctx, `SELECT MAX(op_seq_floor, COALESCE((SELECT MAX(seq) FROM ops WHERE node_id = node_state.node_id), 0)) + 1 FROM node_state WHERE id = 1`).Scan(&op.Seq)
	if err != nil {
		return Op{}, err
	}
	err = tx.QueryRowContext(ctx, `SELECT MAX(
		COALESCE((SELECT MAX(hlc) FROM ops WHERE entity = ? AND entity_id = ?), ''),
		COALESCE((SELECT hlc FROM row_snapshots WHERE entity = ? AND entity_id = ?), ''))`, entity, id, entity, id).Scan(&op.Base)
	if err != nil {
		return Op{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO ops(node_id, seq, hlc, entity, entity_id, fields, base) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		op.Node, op.Seq, op.HLC, op.Entity, op.EntityID, string(op.Fields), op.Base); err != nil {
		return Op{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE node_state SET hlc = ?, op_seq_floor = ? WHERE id = 1`, op.HLC, op.Seq); err != nil {
		return Op{}, err
	}
	return op, nil
}

// Vector returns what this node holds from every origin.
func (l *Log) Vector(ctx context.Context) (Vector, error) {
	rows, err := l.db.QueryContext(ctx, `SELECT node_id, MAX(seq) FROM (
		SELECT node_id, MAX(seq) AS seq FROM ops GROUP BY node_id
		UNION ALL
		SELECT j.key, j.value FROM node_state, json_each(compaction_horizon) j WHERE node_state.id = 1
	) GROUP BY node_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	v := Vector{}
	for rows.Next() {
		var node string
		var seq int64
		if err = rows.Scan(&node, &seq); err != nil {
			return nil, err
		}
		v[node] = seq
	}
	return v, rows.Err()
}

// Since returns up to limit operations that a node with vector since lacks,
// including those of third nodes, in clock order.
func (l *Log) Since(ctx context.Context, since Vector, limit int) ([]Op, error) {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	horizon, err := readHorizon(ctx, tx)
	if err != nil {
		return nil, err
	}
	if !since.Covers(horizon) {
		return nil, ErrSnapshotRequired
	}
	rows, err := tx.QueryContext(ctx, `SELECT node_id, seq, hlc, entity, entity_id, fields, base FROM ops o
		WHERE seq > COALESCE((SELECT value FROM json_each(?) WHERE key = o.node_id), 0)
		ORDER BY hlc, node_id, seq LIMIT ?`, mustVectorJSON(since), limit)
	if err != nil {
		return nil, err
	}
	return scanOps(rows)
}
