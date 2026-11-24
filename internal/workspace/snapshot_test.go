package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTurnSnapshotDiff(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	for _, a := range [][]string{{"init", "-q"}, {"config", "user.email", "a@b"}, {"config", "user.name", "a"}} {
		if _, err := Run(ctx, dir, a...); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("1\n2\n"), 0o644)
	Run(ctx, dir, "add", ".")
	Run(ctx, dir, "commit", "-qm", "x")
	base := Snapshot(ctx, dir)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("1\n3\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("n\n"), 0o644)
	files := TreeChanges(ctx, dir, base, Snapshot(ctx, dir))
	if len(files) != 2 || files[0].Path != "a.txt" || files[0].Added != 1 || files[0].Removed != 1 || files[1].Change != "add" {
		t.Fatalf("got %+v", files)
	}
	if out, _ := Run(ctx, dir, "status", "--porcelain"); !strings.Contains(string(out), "?? new.txt") {
		t.Fatalf("index touched: %s", out)
	}
}

func TestRestoreSnapshot(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	Run(ctx, dir, "init", "-q")
	write := func(name, s string) { os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644) }
	write("a.txt", "1\n")
	write("gone.txt", "g\n")
	root := RepoRoot(ctx, dir)
	KeepSnapshot(ctx, root, "th", "run1", Snapshot(ctx, root))
	write("a.txt", "2\n")
	write("new.txt", "n\n")
	os.Remove(filepath.Join(dir, "gone.txt"))
	snap, ok := SnapshotAt(ctx, root, "th", "", time.Now())
	if !ok {
		t.Fatal("snapshot not found by time")
	}
	if err := RestoreSnapshot(ctx, root, snap.Ref); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(b) != "1\n" {
		t.Fatalf("a.txt = %q", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); !os.IsNotExist(err) {
		t.Fatal("new.txt kept")
	}
	if _, err := os.Stat(filepath.Join(dir, "gone.txt")); err != nil {
		t.Fatal("gone.txt not restored")
	}
	DropSnapshots(ctx, root, "th", 0)
	if _, ok := SnapshotAt(ctx, root, "th", "run1", time.Time{}); ok {
		t.Fatal("ref kept")
	}
}
