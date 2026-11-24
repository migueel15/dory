package oplog

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/migueel15/dory/internal/schema"
)

// RecordPeerVector records a completed exchange with a currently paired peer.
func (l *Log) RecordPeerVector(ctx context.Context, peer string, vector Vector) error {
	raw, err := json.Marshal(vector)
	if err != nil {
		return err
	}
	_, err = l.db.ExecContext(ctx, `INSERT INTO peer_vectors (node_id, vector, synced_at)
		SELECT node_id, ?, ? FROM peers WHERE node_id = ? AND revoked_at IS NULL AND disabled_at IS NULL
		ON CONFLICT(node_id) DO UPDATE SET vector = excluded.vector, synced_at = excluded.synced_at`,
		string(raw), time.Now().UTC().Format(time.RFC3339Nano), peer)
	return err
}

// Compact checkpoints stable prefixes in bounded transactions.
func (l *Log) Compact(ctx context.Context) error {
	known, err := knownEntities()
	if err != nil {
		return err
	}
	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339Nano)
	for {
		n, err := l.compactBatch(ctx, known, cutoff)
		if err != nil || n < BatchSize {
			return err
		}
	}
}

func (l *Log) compactBatch(ctx context.Context, known, cutoff string) (int, error) {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	horizon, err := readHorizon(ctx, tx)
	if err != nil {
		return 0, err
	}
	// Stop before the first unknown operation of an origin. Skipping
	// that sequence would make a bootstrap receiver miss the retained op.
	rows, err := tx.QueryContext(ctx, `WITH active AS (
		SELECT COALESCE(v.vector, '{}') AS vector FROM peers p LEFT JOIN peer_vectors v USING (node_id)
		WHERE p.revoked_at IS NULL AND p.disabled_at IS NULL AND (v.node_id IS NULL OR julianday(v.synced_at) >= julianday(?))
	), unknown AS (
		SELECT node_id, MIN(seq) AS first FROM ops
		WHERE entity NOT IN (SELECT value FROM json_each(?)) GROUP BY node_id
	)
	SELECT o.node_id, o.seq, o.hlc, o.entity, o.entity_id, o.fields, o.base FROM ops o
	LEFT JOIN unknown u ON u.node_id = o.node_id
	WHERE (u.first IS NULL OR o.seq < u.first)
	AND o.seq > COALESCE((SELECT value FROM json_each(?) WHERE key = o.node_id), 0)
	AND NOT EXISTS (SELECT 1 FROM active a WHERE
		o.seq > COALESCE((SELECT value FROM json_each(a.vector) WHERE key = o.node_id), 0))
	ORDER BY o.node_id, o.seq LIMIT ?`, cutoff, known, mustVectorJSON(horizon), BatchSize)
	if err != nil {
		return 0, err
	}
	ops, err := scanOps(rows)
	if err != nil || len(ops) == 0 {
		return 0, err
	}
	snapshots := map[[2]string]*RowSnapshot{}
	for _, op := range ops {
		key := [2]string{op.Entity, op.EntityID}
		s := snapshots[key]
		if s == nil {
			loaded, err := readSnapshot(ctx, tx, op.Entity, op.EntityID)
			if err != nil {
				return 0, err
			}
			s = &loaded
			if s.Horizon == nil {
				s.Horizon = Vector{}
			}
			snapshots[key] = s
		}
		entity, _ := schema.Lookup(op.Entity)
		if err := s.fold(op, entity.Merge); err != nil {
			return 0, err
		}
		s.Horizon[op.Node] = op.Seq
		horizon[op.Node] = op.Seq
	}
	for _, s := range snapshots {
		if err := s.save(ctx, tx); err != nil {
			return 0, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM ops AS o
		WHERE o.seq <= COALESCE((SELECT value FROM json_each(?) WHERE key = o.node_id), 0)
		AND o.entity IN (SELECT value FROM json_each(?))`, mustVectorJSON(horizon), known); err != nil {
		return 0, err
	}
	if err := saveHorizon(ctx, tx, horizon); err != nil {
		return 0, err
	}
	return len(ops), tx.Commit()
}

func mustVectorJSON(v Vector) string {
	raw, _ := json.Marshal(v) // string keys and int64 values always encode
	return string(raw)
}

func scanOps(rows *sql.Rows) ([]Op, error) {
	defer rows.Close()
	ops := []Op{}
	for rows.Next() {
		var op Op
		var fields string
		if err := rows.Scan(&op.Node, &op.Seq, &op.HLC, &op.Entity, &op.EntityID, &fields, &op.Base); err != nil {
			return nil, err
		}
		op.Fields = json.RawMessage(fields)
		ops = append(ops, op)
	}
	return ops, rows.Err()
}
