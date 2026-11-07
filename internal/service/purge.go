package service

import (
	"context"
	"log"
	"time"
)

// PurgeAfter is how long an archived record stays before it is deleted for good.
const PurgeAfter = 14 * 24 * time.Hour

// purgeSteps delete, children first, every record archived before $1. A project
// still referenced by something is skipped and waits. Audit events are kept.
var purgeSteps = []string{
	// Projects archived before the cascade existed: archive their children with them.
	`UPDATE repositories c SET archived_at = p.archived_at, updated_at = now() FROM projects p WHERE c.project_id = p.id AND p.archived_at < $1 AND c.archived_at IS NULL`,
	`UPDATE items c SET archived_at = p.archived_at, updated_at = now() FROM projects p WHERE c.project_id = p.id AND p.archived_at < $1 AND c.archived_at IS NULL`,
	`UPDATE features c SET archived_at = p.archived_at, updated_at = now() FROM projects p WHERE c.project_id = p.id AND p.archived_at < $1 AND c.archived_at IS NULL`,
	`DELETE FROM item_plans WHERE archived_at < $1 OR item_id IN (SELECT id FROM items WHERE archived_at < $1)`,
	`DELETE FROM item_threads WHERE archived_at < $1 OR item_id IN (SELECT id FROM items WHERE archived_at < $1)`,
	`DELETE FROM items WHERE archived_at < $1`,
	`UPDATE items SET feature_id = NULL WHERE feature_id IN (SELECT id FROM features WHERE archived_at < $1)`,
	`DELETE FROM features WHERE archived_at < $1`,
	`DELETE FROM repository_locations WHERE archived_at < $1 OR repository_id IN (SELECT id FROM repositories WHERE archived_at < $1)`,
	`DELETE FROM repositories WHERE archived_at < $1`,
	`DELETE FROM issue_threads WHERE archived_at < $1 OR project_id IN (SELECT id FROM projects WHERE archived_at < $1)`,
	`DELETE FROM projects p WHERE archived_at < $1
		AND NOT EXISTS (SELECT 1 FROM items WHERE project_id = p.id)
		AND NOT EXISTS (SELECT 1 FROM features WHERE project_id = p.id)
		AND NOT EXISTS (SELECT 1 FROM repositories WHERE project_id = p.id)`,
	`DELETE FROM memories WHERE archived_at < $1`,
	`DELETE FROM links WHERE archived_at < $1`,
}

// Purge deletes, in one transaction, what was archived before the cutoff.
func (b *backend) Purge(ctx context.Context, cutoff time.Time) (int64, error) {
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var total int64
	for _, step := range purgeSteps {
		tag, err := tx.Exec(ctx, step, cutoff)
		if err != nil {
			return 0, err
		}
		total += tag.RowsAffected()
	}
	return total, tx.Commit(ctx)
}

// Purger is implemented by the service backend.
type Purger interface {
	Purge(ctx context.Context, cutoff time.Time) (int64, error)
}

// RunPurge deletes old archived records now and then every hour until ctx ends.
func RunPurge(ctx context.Context, p Purger) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		if n, err := p.Purge(ctx, time.Now().Add(-PurgeAfter)); err != nil {
			log.Printf("purge: %v", err)
		} else if n > 0 {
			log.Printf("purge: deleted %d records archived more than %s ago", n, PurgeAfter)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
