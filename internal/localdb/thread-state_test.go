package localdb

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/migueel15/dory/internal/oplog"
	"github.com/pressly/goose/v3"
)

func TestMigrateThreadState(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "dory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, sub)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 25); err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_state (id, node_id) VALUES (1, 'local');
		INSERT INTO threads (id, node_id, provider, status, created_at, updated_at, last_activity_at, read_at)
		VALUES ('t', 'local', 'codex', 'error', '2026-11-24T00:00:00Z', '2026-11-24T00:00:04Z', '2026-11-24T00:00:04Z', '2026-11-24T00:00:02Z');
		INSERT INTO thread_messages (id, thread_id, role, title, detail, created_at, updated_at)
		VALUES ('legacy-tool', 't', 'tool', 'Bash', 'go test ./...', '2026-11-24T00:00:00Z', '2026-11-24T00:00:00Z');
		INSERT INTO ops (node_id, seq, hlc, entity, entity_id, fields) VALUES
		('local', 1, '1', 'thread', 't', '{"node_id":"local","provider":"codex","status":"idle","created_at":"2026-11-24T00:00:00Z","updated_at":"2026-11-24T00:00:00Z"}'),
		('local', 2, '2', 'thread', 't', '{"read_at":"2026-11-24T00:00:02Z","status":"working","updated_at":"2026-11-24T00:00:02Z"}'),
		('local', 3, '3', 'thread', 't', '{"last_activity_at":"2026-11-24T00:00:03Z"}'),
		('local', 4, '4', 'thread', 't', '{"status":"error","last_activity_at":"2026-11-24T00:00:04Z","updated_at":"2026-11-24T00:00:04Z"}');`)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	var kind, command string
	err = db.QueryRowContext(ctx, `SELECT json_extract(tool, '$.kind'), json_extract(tool, '$.command')
		FROM thread_messages WHERE id = 'legacy-tool'`).Scan(&kind, &command)
	if err != nil || kind != "command" || command != "go test ./..." {
		t.Fatalf("migrated tool = %q %q: %v", kind, command, err)
	}
	var count, stateFields int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*),
		SUM(json_type(fields, '$.status') IS NOT NULL OR json_type(fields, '$.last_activity_at') IS NOT NULL)
		FROM ops WHERE entity = 'thread'`).Scan(&count, &stateFields)
	if err != nil || count != 2 || stateFields != 0 {
		t.Fatalf("remaining ops = %d, with state = %d: %v", count, stateFields, err)
	}
	t.Logf("migration removed %d of 4 operations", 4-count)
	var read, updated, status, activity string
	err = db.QueryRowContext(ctx, `SELECT t.read_at, t.updated_at, s.status, s.last_activity_at
		FROM threads t JOIN thread_state s ON s.thread_id = t.id AND s.node_id = t.node_id`).Scan(&read, &updated, &status, &activity)
	if err != nil || read != "2026-11-24T00:00:02Z" || updated != read || status != "error" || activity != "2026-11-24T00:00:04Z" {
		t.Fatalf("migrated thread = %q, %q, %q, %q: %v", read, updated, status, activity, err)
	}
	log, err := oplog.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Tx(ctx, func(tx *oplog.Tx) error {
		op, err := tx.Write("thread", "t", map[string]any{"title": "Migrado"})
		if err == nil && op.Seq != 5 {
			t.Errorf("sequence = %d, want 5", op.Seq)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
