package api

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCleanBranch(t *testing.T) {
	for in, want := range map[string]string{
		"Feat/Add Login Page\nextra": "feat/add-login-page",
		"`fix/bug..x`":               "fix/bug.x",
		"":                           "",
		"///":                        "",
	} {
		if got := cleanBranch(in); got != want {
			t.Errorf("cleanBranch(%q) = %q, want %q", in, got, want)
		}
	}
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=a@b.c", "-c", "user.name=t"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s", args, out)
	}
}

func TestPullBranch(t *testing.T) {
	root := t.TempDir()
	origin, clone := filepath.Join(root, "origin"), filepath.Join(root, "clone")
	if err := os.Mkdir(origin, 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, origin, "init", "-b", "main")
	run(t, origin, "commit", "--allow-empty", "-m", "one")
	run(t, root, "clone", origin, clone)
	run(t, origin, "commit", "--allow-empty", "-m", "two")
	ctx := context.Background()

	// Uncommitted changes keep the checkout as it is.
	if err := os.WriteFile(filepath.Join(clone, "wip"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := pullBranch(ctx, clone, "main"); err != nil {
		t.Fatal(err)
	}
	if out, _ := gitOut(ctx, clone, "log", "--oneline"); len(out) == 0 || countLines(out) != 1 {
		t.Fatalf("pulled with a dirty tree: %s", out)
	}
	os.Remove(filepath.Join(clone, "wip"))
	// Another branch is left alone.
	if err := pullBranch(ctx, clone, "other"); err != nil {
		t.Fatal(err)
	}
	if err := pullBranch(ctx, clone, "main"); err != nil {
		t.Fatal(err)
	}
	if out, _ := gitOut(ctx, clone, "log", "--oneline"); countLines(out) != 2 {
		t.Fatalf("not fast-forwarded: %s", out)
	}
}

func countLines(s string) int {
	n := 1
	for _, c := range s {
		if c == '\n' {
			n++
		}
	}
	return n
}
