package workspace

import (
	"context"
	"strings"

	"github.com/migueel15/dory/internal/cmdexec"
)

// WorkDiff is the diff of a folder and what it was compared with: Base is the
// reference chosen ("" for the uncommitted changes, HEAD), and Branches are
// the local branches it can be compared with instead.
type WorkDiff struct {
	Diff     string   `json:"diff"`
	Base     string   `json:"base"`
	Branches []string `json:"branches"`
}

// Diff compares the folder dir with against: a branch, from the point the work
// left it (merge-base), or "HEAD" for only what is not committed. Empty
// against uses base, the branch the thread started from.
func Diff(ctx context.Context, dir, against, base string) (WorkDiff, error) {
	out := WorkDiff{Branches: []string{}}
	if _, err := Output(ctx, dir, "rev-parse", "--git-dir"); err != nil {
		return out, nil
	}
	if refs, err := Output(ctx, dir, "for-each-ref", "--format=%(refname:short)", "refs/heads"); err == nil {
		out.Branches = append(out.Branches, strings.Fields(refs)...)
	}
	if against == "" {
		against = base
	}
	if against == "" || strings.HasPrefix(against, "-") {
		against = "HEAD"
	}
	out.Base = against
	ref := "HEAD"
	if against != "HEAD" {
		mb, err := Output(ctx, dir, "merge-base", against, "HEAD")
		if err != nil {
			// An unknown reference falls back to the uncommitted changes.
			out.Base = "HEAD"
		} else {
			ref = mb
		}
	}
	// Against a snapshot of the folder, so new files not yet added count too.
	args := []string{"diff", ref}
	if tree := Snapshot(ctx, RepoRoot(ctx, dir)); tree != "" {
		args = append(args, tree)
	}
	diff, err := cmdexec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	out.Diff = string(diff)
	return out, err
}
