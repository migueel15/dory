package api

import (
	"context"
	"os/exec"
	"slices"
	"testing"
)

func TestBranches(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "x"},
		{"branch", "feat/a"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	b := branches(context.Background(), dir)
	if b.Current != "main" || !slices.Contains(b.Branches, "main") || !slices.Contains(b.Branches, "feat/a") {
		t.Fatalf("branches = %+v", b)
	}
}
