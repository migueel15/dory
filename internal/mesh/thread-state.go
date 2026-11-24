package mesh

import (
	"context"
	"database/sql"
	"time"

	"github.com/migueel15/dory/internal/data"
)

// ThreadStateChanged coalesces a burst of local turn updates into one mesh wake.
func (m *Mesh) ThreadStateChanged() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.threadWake == nil {
		m.threadWake = time.AfterFunc(time.Second, func() {
			m.mu.Lock()
			m.threadWake = nil
			m.mu.Unlock()
			m.Changed()
		})
	}
}

func (m *Mesh) ownedThreadStates(ctx context.Context) ([]data.ThreadState, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT t.id, COALESCE(s.status, 'idle'), COALESCE(s.error, ''), s.last_activity_at
		FROM threads t LEFT JOIN thread_state s ON s.thread_id = t.id AND s.node_id = t.node_id
		WHERE t.node_id = ? AND t.archived_at IS NULL ORDER BY t.id`, m.log.Node())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []data.ThreadState{}
	for rows.Next() {
		var s data.ThreadState
		if err = rows.Scan(&s.ID, &s.Status, &s.Error, &s.LastActivityAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (m *Mesh) cacheThreadStates(ctx context.Context, tx *sql.Tx, node, at string, states []data.ThreadState) ([]string, error) {
	var changed []string
	if node == m.log.Node() {
		return changed, nil
	}
	for _, s := range states {
		// Info can arrive before metadata. Prefer the owner in threads when
		// known, and keep any existing state from other nodes otherwise.
		res, err := tx.ExecContext(ctx, `INSERT INTO thread_state (thread_id, node_id, status, error, last_activity_at, updated_at)
			SELECT ?, ?, ?, ?, ?, ?
			WHERE NOT EXISTS (SELECT 1 FROM threads WHERE id = ? AND node_id <> ?)
			ON CONFLICT(thread_id) DO UPDATE SET node_id = excluded.node_id, status = excluded.status,
				error = excluded.error, last_activity_at = excluded.last_activity_at, updated_at = excluded.updated_at
			WHERE (thread_state.node_id = excluded.node_id OR EXISTS (
				SELECT 1 FROM threads WHERE id = excluded.thread_id AND node_id = excluded.node_id))
			AND (thread_state.node_id <> excluded.node_id OR thread_state.status <> excluded.status
				OR thread_state.error <> excluded.error OR thread_state.last_activity_at IS NOT excluded.last_activity_at)`,
			s.ID, node, s.Status, s.Error, s.LastActivityAt, at, s.ID, node)
		if err != nil {
			return nil, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}
		if n > 0 {
			changed = append(changed, s.ID)
		}
	}
	return changed, nil
}
