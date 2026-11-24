package oplog

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/migueel15/dory/internal/schema"
)

// Tx reads and writes the materialized state within a log transaction.
type Tx struct {
	log     *Log
	ctx     context.Context
	sql     *sql.Tx
	changes []Op
	err     error
}

// Tx runs fn in a writer transaction. localdb configures BEGIN IMMEDIATE so
// reads used to decide what to write are serialized with other writers.
func (l *Log) Tx(ctx context.Context, fn func(*Tx) error) error {
	sqlTx, err := l.begin(ctx)
	if err != nil {
		return err
	}
	defer sqlTx.Rollback()
	tx := &Tx{log: l, ctx: ctx, sql: sqlTx}
	if err = fn(tx); err != nil {
		return err
	}
	if tx.err != nil {
		return tx.err
	}
	if err = sqlTx.Commit(); err != nil {
		return err
	}
	if onChange := l.OnChange; onChange != nil {
		for _, op := range tx.changes {
			onChange(op.Entity, op.EntityID)
		}
	}
	return nil
}

func (tx *Tx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return tx.sql.QueryContext(ctx, query, args...)
}

// ExecContext writes local state in the same transaction as the log.
func (tx *Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return tx.sql.ExecContext(ctx, query, args...)
}

// Write records and materializes one operation. A failed write aborts the
// transaction even if the callback ignores the error.
func (tx *Tx) Write(entity, id string, fields map[string]any) (Op, error) {
	if tx.err != nil {
		return Op{}, tx.err
	}
	op, err := tx.write(entity, id, fields)
	if err != nil {
		tx.err = err
		return Op{}, err
	}
	tx.changes = append(tx.changes, op)
	return op, nil
}

func (tx *Tx) write(entity, id string, fields map[string]any) (Op, error) {
	t, ok := schema.Lookup(entity)
	if !ok {
		return Op{}, fmt.Errorf("oplog: unknown entity %q", entity)
	}
	for name := range fields {
		if _, ok := t.Column(name); !ok {
			return Op{}, fmt.Errorf("oplog: %s has no column %q", entity, name)
		}
	}
	if remote, ok := fields["remote_url"].(string); ok && entity == "repository" && id != RepositoryID(remote) {
		return Op{}, fmt.Errorf("oplog: repository id must be RepositoryID(%q)", remote)
	}
	op, err := tx.log.Append(tx.ctx, tx.sql, entity, id, fields)
	if err != nil {
		return Op{}, err
	}
	return op, materialize(tx.ctx, tx.sql, entity, id)
}

// Write records and applies a single local change.
func (l *Log) Write(ctx context.Context, entity, id string, fields map[string]any) (Op, error) {
	var op Op
	err := l.Tx(ctx, func(tx *Tx) error {
		var err error
		op, err = tx.Write(entity, id, fields)
		return err
	})
	return op, err
}
