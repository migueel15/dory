package data

import (
	"context"
	"time"

	"github.com/migueel15/dory/internal/oplog"
	"github.com/migueel15/dory/internal/schema"
)

// PeerConnectedSQL is shared by thread reads and the mesh's peer list.
const PeerConnectedSQL = `peers.revoked_at IS NULL AND peers.disabled_at IS NULL
	AND peers.last_error NOT GLOB 'incompatible_protocol:*'
	AND julianday(peers.last_seen) > julianday('now', '-30 seconds')`

// ThreadState is the last turn state advertised by the node that owns a thread.
type ThreadState struct {
	ID             string  `json:"id"`
	Status         string  `json:"status"`
	Error          string  `json:"error"`
	LastActivityAt *string `json:"last_activity_at"`
}

// SetThreadState changes the runtime state of a thread owned by this node.
func (s *Store) SetThreadState(ctx context.Context, id, status, failure string) error {
	at := s.now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx, `INSERT INTO thread_state (thread_id, node_id, status, error, last_activity_at, updated_at)
		SELECT id, node_id, ?, ?, ?, ? FROM threads WHERE id = ? AND node_id = ?
		ON CONFLICT(thread_id) DO UPDATE SET node_id = excluded.node_id, status = excluded.status,
			error = excluded.error, last_activity_at = excluded.last_activity_at, updated_at = excluded.updated_at`,
		status, failure, at, at, id, s.log.Node())
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

func threadUnread(activity, read *string) bool {
	if activity == nil {
		return false
	}
	if read == nil {
		return true
	}
	at, activityErr := time.Parse(time.RFC3339Nano, *activity)
	seen, readErr := time.Parse(time.RFC3339Nano, *read)
	return activityErr == nil && readErr == nil && at.After(seen)
}

// MarkThreadRead records a read only if the thread has unread activity.
func (s *Store) MarkThreadRead(ctx context.Context, id string) (Thread, error) {
	r := s.Threads()
	var t Thread
	spec, _ := schema.Lookup("thread")
	err := s.log.Tx(ctx, func(tx *oplog.Tx) error {
		raw, err := r.row(ctx, tx, id, false)
		if err != nil {
			return err
		}
		t, err = decode[Thread](raw)
		if err != nil || !t.Unread {
			return err
		}
		t, err = r.update(ctx, tx, spec, id, raw, nil, Fields{"read_at": s.now().UTC().Format(time.RFC3339Nano)})
		return err
	})
	return writeResult(t, err)
}
