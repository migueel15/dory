package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTurnSnapshotDiff(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	for _, a := range [][]string{{"init", "-q"}, {"config", "user.email", "a@b"}, {"config", "user.name", "a"}} {
		if _, err := gitCmd(ctx, dir, a...); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("1\n2\n"), 0o644)
	gitCmd(ctx, dir, "add", ".")
	gitCmd(ctx, dir, "commit", "-qm", "x")
	base := snapshot(ctx, dir)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("1\n3\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("n\n"), 0o644)
	files := treeChanges(ctx, dir, base, snapshot(ctx, dir))
	if len(files) != 2 || files[0].Path != "a.txt" || files[0].Added != 1 || files[0].Removed != 1 || files[1].Change != "add" {
		t.Fatalf("got %+v", files)
	}
	if out, _ := gitCmd(ctx, dir, "status", "--porcelain"); !strings.Contains(string(out), "?? new.txt") {
		t.Fatalf("index touched: %s", out)
	}
}
