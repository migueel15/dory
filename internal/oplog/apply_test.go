package oplog

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/localdb"
)

func node(t *testing.T) (*sql.DB, *Log) {
	t.Helper()
	ctx := context.Background()
	db, err := localdb.Open(ctx, filepath.Join(t.TempDir(), "dory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	log, err := Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	return db, log
}

func write(t *testing.T, l *Log, entity, id string, fields map[string]any) {
	t.Helper()
	if _, err := l.Write(context.Background(), entity, id, fields); err != nil {
		t.Fatal(err)
	}
}

// sync hands from to the ops it is missing, as a peer exchange would.
func sync(t *testing.T, from, to *Log) {
	t.Helper()
	ctx := context.Background()
	v, err := to.Vector(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ops, err := from.Since(ctx, v, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if err = to.Apply(ctx, ops); err != nil {
		t.Fatal(err)
	}
}

func task(t *testing.T, db *sql.DB) (title, description string) {
	t.Helper()
	if err := db.QueryRow(`SELECT title, description FROM tasks WHERE id = 't1'`).Scan(&title, &description); err != nil {
		t.Fatal(err)
	}
	return title, description
}

func TestOfflineEditsConverge(t *testing.T) {
	dbA, a := node(t)
	dbB, b := node(t)
	now := "2026-11-16T00:00:00Z"
	write(t, a, "project", "p1", map[string]any{"slug": "dory", "name": "Dory", "created_at": now, "updated_at": now})
	write(t, a, "task", "t1", map[string]any{"project_id": "p1", "title": "Inicial", "issue_number": 148, "created_at": now, "updated_at": now})
	sync(t, a, b)
	if title, _ := task(t, dbB); title != "Inicial" {
		t.Fatalf("b did not receive the task: %q", title)
	}

	// Both edit offline: a changes the title, then b changes title and description.
	write(t, a, "task", "t1", map[string]any{"title": "De A"})
	write(t, b, "task", "t1", map[string]any{"title": "De B", "description": "Solo B"})
	sync(t, a, b)
	sync(t, b, a)
	sync(t, a, b) // a second round must change nothing

	titleA, descA := task(t, dbA)
	titleB, descB := task(t, dbB)
	if titleA != titleB || descA != descB {
		t.Fatalf("diverged: a=(%q,%q) b=(%q,%q)", titleA, descA, titleB, descB)
	}
	// Concurrent edits of the same field: either may win, but both nodes agree.
	// Edits of different fields are both kept.
	if (titleA != "De A" && titleA != "De B") || descA != "Solo B" {
		t.Fatalf("unexpected merge: (%q,%q)", titleA, descA)
	}

	// An edit made after seeing the other one always wins.
	write(t, a, "task", "t1", map[string]any{"title": "Final"})
	sync(t, a, b)
	write(t, b, "task", "t1", map[string]any{"title": "Final de B"})
	sync(t, b, a)
	if title, _ := task(t, dbA); title != "Final de B" {
		t.Fatalf("later edit lost: %q", title)
	}
	var issue int
	dbA.QueryRow(`SELECT issue_number FROM tasks WHERE id = 't1'`).Scan(&issue)
	if issue != 148 {
		t.Fatalf("untouched field lost: issue %d", issue)
	}
}

func TestThirdNodeGetsOpsThroughRelay(t *testing.T) {
	_, a := node(t)
	_, server := node(t)
	dbC, c := node(t)
	now := "2026-11-16T00:00:00Z"
	write(t, a, "memory", "m1", map[string]any{"scope": "global", "content": "hola", "labels": []any{"v2"}, "created_at": now, "updated_at": now})
	sync(t, a, server)
	sync(t, server, c)
	var content, labels string
	if err := dbC.QueryRow(`SELECT content, labels FROM memories WHERE id = 'm1'`).Scan(&content, &labels); err != nil {
		t.Fatal(err)
	}
	if content != "hola" || labels != `["v2"]` {
		t.Fatalf("got %q %q", content, labels)
	}
}

func TestCompositeKeyAndRefusals(t *testing.T) {
	dbA, a := node(t)
	now := "2026-11-16T00:00:00Z"
	for _, id := range []string{"t1", "t2"} {
		write(t, a, "task", id, map[string]any{"title": id, "created_at": now, "updated_at": now})
	}
	write(t, a, "task_dependency", "t2/t1", map[string]any{"created_at": now})
	var n int
	dbA.QueryRow(`SELECT count(*) FROM task_dependencies WHERE task_id = 't2' AND depends_on_id = 't1'`).Scan(&n)
	if n != 1 {
		t.Fatalf("dependency not written")
	}
	ctx := context.Background()
	if _, err := a.Write(ctx, "task", "t1", map[string]any{"title; DROP TABLE tasks": "x"}); err == nil {
		t.Fatal("unknown column accepted")
	}
	if _, err := a.Write(ctx, "user", "u1", map[string]any{"username": "x"}); err == nil {
		t.Fatal("local table accepted as replicated")
	}
	if _, err := a.Write(ctx, "task", "t3", map[string]any{"title": "x", "project_id": "missing", "created_at": now, "updated_at": now}); err == nil {
		t.Fatal("missing parent accepted")
	}
}

func slugs(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	rows, err := db.Query(`SELECT id, slug FROM projects`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var id, slug string
		rows.Scan(&id, &slug)
		got[id] = slug
	}
	return got
}

func TestDuplicateSlugsRenameTheSameOnEveryNode(t *testing.T) {
	dbA, a := node(t)
	dbB, b := node(t)
	now := "2026-11-16T00:00:00Z"
	// b's project has the larger id, so it is the one renamed, whichever arrives first.
	write(t, b, "project", "pb", map[string]any{"slug": "dory", "name": "B", "created_at": now, "updated_at": now})
	write(t, a, "project", "pa", map[string]any{"slug": "dory", "name": "A", "created_at": now, "updated_at": now})
	sync(t, a, b)
	sync(t, b, a)
	want := map[string]string{"pa": "dory", "pb": "dory-pb"}
	for name, db := range map[string]*sql.DB{"a": dbA, "b": dbB} {
		if got := slugs(t, db); got["pa"] != want["pa"] || got["pb"] != want["pb"] {
			t.Fatalf("node %s: %v", name, got)
		}
	}
}

func TestSameRepositoryFromTwoNodesIsOneRow(t *testing.T) {
	dbA, a := node(t)
	_, b := node(t)
	now := "2026-11-16T00:00:00Z"
	for _, n := range []struct {
		log    *Log
		remote string
	}{{a, "https://github.com/migueel15/dory.git"}, {b, "git@github.com:migueel15/dory"}} {
		write(t, n.log, "project", "p1", map[string]any{"slug": "dory", "name": "Dory", "created_at": now, "updated_at": now})
		write(t, n.log, "repository", RepositoryID(n.remote), map[string]any{"project_id": "p1", "remote_url": n.remote, "created_at": now, "updated_at": now})
	}
	sync(t, b, a)
	var n int
	dbA.QueryRow(`SELECT count(*) FROM repositories`).Scan(&n)
	if n != 1 {
		t.Fatalf("got %d repositories", n)
	}
	if _, err := a.Write(context.Background(), "repository", "other", map[string]any{"remote_url": "https://github.com/x/y"}); err == nil {
		t.Fatal("repository id not derived from remote accepted")
	}
}

func memory(t *testing.T, db *sql.DB) string {
	t.Helper()
	var content string
	if err := db.QueryRow(`SELECT content FROM memories WHERE id = 'm1'`).Scan(&content); err != nil {
		t.Fatal(err)
	}
	return content
}

func TestConcurrentMemoryEditsKeepBothTexts(t *testing.T) {
	dbA, a := node(t)
	dbB, b := node(t)
	now := "2026-11-16T00:00:00Z"
	write(t, a, "memory", "m1", map[string]any{"scope": "global", "content": "base", "created_at": now, "updated_at": now})
	sync(t, a, b)
	write(t, a, "memory", "m1", map[string]any{"content": "texto de A"})
	write(t, b, "memory", "m1", map[string]any{"content": "texto de B"})
	sync(t, a, b)
	sync(t, b, a)
	got := memory(t, dbA)
	if got != memory(t, dbB) {
		t.Fatalf("diverged:\n%s\n---\n%s", got, memory(t, dbB))
	}
	if !strings.Contains(got, "texto de A") || !strings.Contains(got, "texto de B") || strings.Contains(got, "base") {
		t.Fatalf("lost or stale text:\n%s", got)
	}

	// Saving after seeing the conflict resolves it.
	write(t, a, "memory", "m1", map[string]any{"content": "resuelto"})
	sync(t, a, b)
	if got = memory(t, dbB); got != "resuelto" {
		t.Fatalf("conflict not resolved: %q", got)
	}
}
