package oplog

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/migueel15/dory/internal/localdb"
)

func TestAppendAndSince(t *testing.T) {
	ctx := context.Background()
	db, err := localdb.Open(ctx, filepath.Join(t.TempDir(), "dory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	log, err := Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"uno", "dos"} {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = log.Append(ctx, tx, "task", "t1", map[string]any{"title": title}); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	vector, err := log.Vector(ctx)
	if err != nil || vector[log.Node()] != 2 {
		t.Fatalf("vector %v, %v", vector, err)
	}
	ops, err := log.Since(ctx, Vector{log.Node(): 1}, 10)
	if err != nil || len(ops) != 1 || ops[0].Seq != 2 || string(ops[0].Fields) != `{"title":"dos"}` {
		t.Fatalf("since: %+v, %v", ops, err)
	}
	if ops, _ = log.Since(ctx, Vector{}, 10); len(ops) != 2 || !(ops[0].HLC < ops[1].HLC) {
		t.Fatalf("all ops out of order: %+v", ops)
	}

	// The clock resumes after the stored state when the node restarts.
	again, err := Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if next := again.clock.Now().String(); next <= ops[1].HLC {
		t.Fatalf("clock went back: %s <= %s", next, ops[1].HLC)
	}
}

func TestFullTextSearch(t *testing.T) {
	ctx := context.Background()
	db, err := localdb.Open(ctx, filepath.Join(t.TempDir(), "dory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.ExecContext(ctx, `INSERT INTO tasks(id, title, description, created_at, updated_at) VALUES ('t1', 'Migrar a SQLite', 'registro de operaciones', 'x', 'x')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE tasks SET title = 'Nodo local' WHERE id = 't1'`); err != nil {
		t.Fatal(err)
	}
	var n int
	db.QueryRowContext(ctx, `SELECT count(*) FROM tasks_fts WHERE tasks_fts MATCH 'nodo'`).Scan(&n)
	var old int
	db.QueryRowContext(ctx, `SELECT count(*) FROM tasks_fts WHERE tasks_fts MATCH 'migrar'`).Scan(&old)
	if n != 1 || old != 0 {
		t.Fatalf("fts: new=%d old=%d", n, old)
	}
}
