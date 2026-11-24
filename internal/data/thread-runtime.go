package data

import (
	"context"

	"github.com/migueel15/dory/internal/oplog"
)

// CreateLocalThread inserts the replicated thread and registers its local
// runtime atomically. The oplog publishes the thread after both writes commit.
func (s *Store) CreateLocalThread(ctx context.Context, t Thread, register func(*oplog.Tx, Thread) error) (Thread, error) {
	return s.Threads().save(ctx, t, false, register)
}

func (s *Store) Thread(ctx context.Context, id string) (Thread, error) {
	return s.Threads().Get(ctx, id)
}

func (s *Store) ArchiveThread(ctx context.Context, id string) error {
	return s.Threads().Archive(ctx, id)
}
