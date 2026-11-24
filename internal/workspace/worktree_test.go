package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// repo makes a repository with one commit on main.
func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "base")
	return dir
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	args = append([]string{"-c", "user.email=a@b.c", "-c", "user.name=t"}, args...)
	if out, err := Run(context.Background(), dir, args...); err != nil {
		t.Fatalf("git %v: %s", args, out)
	}
}

func TestBranches(t *testing.T) {
	ctx := context.Background()
	dir := repo(t)
	git(t, dir, "branch", "feat/a")
	b := ListBranches(ctx, dir)
	if b.Current != "main" || !slices.Contains(b.Branches, "main") || !slices.Contains(b.Branches, "feat/a") {
		t.Fatalf("branches = %+v", b)
	}
	if err := SwitchBranch(ctx, dir, "feat/a"); err != nil || ListBranches(ctx, dir).Current != "feat/a" {
		t.Fatalf("switch: %v", err)
	}
	if !BranchExists(ctx, dir, "main") || BranchExists(ctx, dir, "nope") {
		t.Fatal("BranchExists")
	}
}

func TestAddWorktree(t *testing.T) {
	ctx := context.Background()
	r := repo(t)
	dir := filepath.Join(t.TempDir(), "deep", "wt1")
	if err := AddWorktree(ctx, r, dir, "feat/new", "main"); err != nil {
		t.Fatal(err)
	}
	if b, _ := CurrentBranch(ctx, dir); b != "feat/new" {
		t.Fatalf("branch %q", b)
	}
	if !IsLinkedWorktree(dir, "wt1") || IsLinkedWorktree(dir, "other") || IsLinkedWorktree(r, filepath.Base(r)) {
		t.Fatal("IsLinkedWorktree")
	}
	if wts := Worktrees(ctx, r); len(wts) != 1 || wts[0].Branch != "feat/new" {
		t.Fatalf("worktrees %+v", wts)
	}
	// An existing branch is reused.
	git(t, r, "branch", "feat/old")
	dir2 := filepath.Join(t.TempDir(), "wt2")
	if err := AddWorktree(ctx, r, dir2, "feat/old", ""); err != nil {
		t.Fatal(err)
	}
	if b, _ := CurrentBranch(ctx, dir2); b != "feat/old" {
		t.Fatalf("branch %q", b)
	}
	// A branch already checked out elsewhere fails with git's message.
	if err := AddWorktree(ctx, r, filepath.Join(t.TempDir(), "wt3"), "feat/old", ""); err == nil || !strings.HasPrefix(err.Error(), "git worktree:") {
		t.Fatalf("err %v", err)
	}
}

func TestRemoveWorktree(t *testing.T) {
	ctx := context.Background()
	r := repo(t)
	dir := filepath.Join(t.TempDir(), "wt")
	if err := AddWorktree(ctx, r, dir, "feat/x", "main"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "wip.txt"), []byte("x"), 0o644)
	if err := RemoveWorktree(ctx, dir, false); !errors.Is(err, ErrDirtyWorktree) {
		t.Fatalf("dirty: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("dirty worktree removed")
	}
	if err := RemoveWorktree(ctx, dir, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("forced removal kept it")
	}
	dir2 := filepath.Join(t.TempDir(), "wt2")
	if err := AddWorktree(ctx, r, dir2, "feat/y", "main"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveWorktree(ctx, dir2, false); err != nil {
		t.Fatalf("clean: %v", err)
	}
}

func TestDiff(t *testing.T) {
	ctx := context.Background()
	r := repo(t)
	os.WriteFile(filepath.Join(r, "a.txt"), []byte("1\n"), 0o644)
	git(t, r, "add", ".")
	git(t, r, "commit", "-q", "-m", "a")
	git(t, r, "checkout", "-q", "-b", "feat")
	os.WriteFile(filepath.Join(r, "b.txt"), []byte("committed\n"), 0o644)
	git(t, r, "add", ".")
	git(t, r, "commit", "-q", "-m", "b")
	os.WriteFile(filepath.Join(r, "c.txt"), []byte("untracked\n"), 0o644) // never added

	d, err := Diff(ctx, r, "", "main")
	if err != nil || d.Base != "main" || !strings.Contains(d.Diff, "+committed") || !strings.Contains(d.Diff, "+untracked") {
		t.Fatalf("against base: %+v %v", d, err)
	}
	if !slices.Contains(d.Branches, "main") || !slices.Contains(d.Branches, "feat") {
		t.Fatalf("branches %v", d.Branches)
	}
	d, _ = Diff(ctx, r, "HEAD", "main")
	if d.Base != "HEAD" || strings.Contains(d.Diff, "+committed") || !strings.Contains(d.Diff, "+untracked") {
		t.Fatalf("against HEAD: %+v", d)
	}
	if d, _ = Diff(ctx, r, "no-such-ref", ""); d.Base != "HEAD" {
		t.Fatalf("unknown ref: %+v", d)
	}
	if d, err = Diff(ctx, t.TempDir(), "", ""); err != nil || d.Diff != "" || d.Branches == nil {
		t.Fatalf("not a repository: %+v %v", d, err)
	}
}

func TestPull(t *testing.T) {
	root := t.TempDir()
	origin, clone := filepath.Join(root, "origin"), filepath.Join(root, "clone")
	os.Mkdir(origin, 0o755)
	git(t, origin, "init", "-b", "main")
	git(t, origin, "commit", "--allow-empty", "-m", "one")
	git(t, root, "clone", origin, clone)
	git(t, origin, "commit", "--allow-empty", "-m", "two")
	ctx := context.Background()
	count := func() int {
		out, _ := Output(ctx, clone, "log", "--oneline")
		return len(strings.Split(out, "\n"))
	}
	// Uncommitted changes keep the checkout as it is.
	os.WriteFile(filepath.Join(clone, "wip"), []byte("x"), 0o644)
	if err := Pull(ctx, clone, "main"); err != nil || count() != 1 {
		t.Fatalf("pulled with a dirty tree: %v", err)
	}
	os.Remove(filepath.Join(clone, "wip"))
	// Another branch is left alone.
	if err := Pull(ctx, clone, "other"); err != nil || count() != 1 {
		t.Fatal(err)
	}
	if err := Pull(ctx, clone, "main"); err != nil || count() != 2 {
		t.Fatalf("not fast-forwarded: %v", err)
	}
}

func TestValidBranchName(t *testing.T) {
	if !ValidBranchName("feat/a") || ValidBranchName("a..b") {
		t.Fatal("ValidBranchName")
	}
}
