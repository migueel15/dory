package data

import (
	"context"
	"time"

	"github.com/migueel15/dory/internal/oplog"
	"github.com/migueel15/dory/internal/schema"
)

// ArchiveReadNotices applies retention through the replicated log.
func (s *Store) ArchiveReadNotices(ctx context.Context, cutoff time.Time) error {
	return s.log.Tx(ctx, func(tx *oplog.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id FROM notices WHERE archived_at IS NULL AND read_at < ?`, cutoff.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		defer rows.Close()
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		spec, _ := schema.Lookup("notice")
		fields := Fields{"archived_at": s.now().UTC().Format(time.RFC3339Nano)}
		for _, id := range ids {
			if _, err := s.Notices().patchTx(ctx, tx, spec, id, nil, fields); err != nil {
				return err
			}
		}
		return nil
	})
}
