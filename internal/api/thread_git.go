package api

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/service"
)

// localChanges is a thread's diff computed by Dory with git, for threads whose
// worktree T3 does not know (the agent created it by hand).
type localChanges struct {
	Diff     string `json:"diff"`
	Source   string `json:"source"` // "t3" or "git"
	Branch   string `json:"branch,omitempty"`
	Worktree string `json:"worktree,omitempty"`
	Base     string `json:"base,omitempty"`
}

// maxUntracked caps how many new, untracked files are added to the diff.
const maxUntracked = 200

func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	// Dory may run with a read-only home: never take the index lock.
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.String(), err
}

// itemBranches returns the branches named in the refs of the thread's items
// ("branch:<name>"), in order.
func itemBranches(ctx context.Context, svc service.Service, links []service.ItemThread) []string {
	var out []string
	for _, l := range links {
		it, err := svc.Items().Get(ctx, l.ItemID)
		if err != nil {
			continue
		}
		for _, r := range it.Refs {
			if b, ok := strings.CutPrefix(r, "branch:"); ok && strings.TrimSpace(b) != "" {
				out = append(out, strings.TrimSpace(b))
			}
		}
	}
	return out
}

// findWorktree looks for a checkout of branch among the worktrees of every
// repository location present on this machine.
func findWorktree(ctx context.Context, svc service.Service, branch string) string {
	locs, err := svc.RepositoryLocations().List(ctx, service.Filter{Limit: 200})
	if err != nil {
		return ""
	}
	want := "branch refs/heads/" + branch
	seen := map[string]bool{}
	for _, l := range locs {
		if l.Path == "" || seen[l.Path] {
			continue
		}
		seen[l.Path] = true
		if st, err := os.Stat(l.Path); err != nil || !st.IsDir() {
			continue
		}
		out, err := git(ctx, l.Path, "worktree", "list", "--porcelain")
		if err != nil {
			continue
		}
		var path string
		for _, line := range strings.Split(out, "\n") {
			if p, ok := strings.CutPrefix(line, "worktree "); ok {
				path = p
			} else if line == want {
				return path
			}
		}
	}
	return ""
}

// worktreeDiff diffs dir against where its branch left the default branch,
// including uncommitted and untracked changes, like T3's worktree diff.
func worktreeDiff(ctx context.Context, dir string) (diff, base string, err error) {
	base = "origin/HEAD"
	if ref, e := git(ctx, dir, "rev-parse", "--abbrev-ref", "origin/HEAD"); e == nil && strings.TrimSpace(ref) != "" {
		base = strings.TrimSpace(ref)
	}
	mb, err := git(ctx, dir, "merge-base", base, "HEAD")
	if err != nil {
		return "", "", err
	}
	diff, err = git(ctx, dir, "diff", "--no-color", "--no-ext-diff", strings.TrimSpace(mb))
	if err != nil {
		return "", "", err
	}
	untracked, _ := git(ctx, dir, "ls-files", "--others", "--exclude-standard", "-z")
	var b strings.Builder
	b.WriteString(diff)
	for i, f := range strings.Split(strings.TrimRight(untracked, "\x00"), "\x00") {
		if f == "" || i >= maxUntracked {
			break
		}
		// --no-index exits 1 when the files differ, which is always here.
		out, _ := git(ctx, dir, "diff", "--no-color", "--no-index", "--", os.DevNull, filepath.ToSlash(f))
		b.WriteString(out)
	}
	return b.String(), strings.TrimPrefix(base, "origin/"), nil
}

// gitChanges finds the thread's worktree from its items' branches and diffs it.
// ok is false when no local worktree is found.
func gitChanges(ctx context.Context, svc service.Service, links []service.ItemThread) (localChanges, bool) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for _, branch := range itemBranches(ctx, svc, links) {
		dir := findWorktree(ctx, svc, branch)
		if dir == "" {
			continue
		}
		diff, base, err := worktreeDiff(ctx, dir)
		if err != nil {
			continue
		}
		return localChanges{Diff: diff, Source: "git", Branch: branch, Worktree: dir, Base: base}, true
	}
	return localChanges{}, false
}
