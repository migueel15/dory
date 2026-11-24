// Package localdb opens the SQLite database of a Dory v2 node.
package localdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Open opens or creates the database at path, applies pending migrations and
// gives the node an identity on first use.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite allows a single writer; one connection avoids lock errors.
	db.SetMaxOpenConns(1)
	if err = migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO node_state(id, node_id) VALUES (1, ?) ON CONFLICT(id) DO NOTHING`, rand.Text()); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, sub)
	if err != nil {
		return err
	}
	if _, err = provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}
