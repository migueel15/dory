package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/apperr"
)

// ErrDirtyWorktree says a worktree holds changes that are not committed.
var ErrDirtyWorktree = apperr.New(apperr.DirtyWorktree, "the worktree has uncommitted changes")

// RepoRoot returns the top folder of the repository dir is in, or "".
func RepoRoot(ctx context.Context, dir string) string {
	if dir == "" {
		return ""
	}
	out, err := Output(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return out
}

// BranchExists says whether repo already has a local branch called name.
func BranchExists(ctx context.Context, repo, name string) bool {
	if repo == "" {
		return false
	}
	_, err := Run(ctx, repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

// AddWorktree creates a worktree of repo at dir on branch. A branch that does
// not exist is created, from base when it is given.
func AddWorktree(ctx context.Context, repo, dir, branch, base string) error {
	_, err := addWorktree(ctx, repo, dir, branch, base)
	return err
}

// addWorktree reports whether git created the branch, so callers can undo it.
func addWorktree(ctx context.Context, repo, dir, branch, base string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return false, err
	}
	args := []string{"worktree", "add"}
	created := !BranchExists(ctx, repo, branch)
	if !created {
		args = append(args, dir, branch)
	} else {
		args = append(args, "-b", branch, dir)
		if base != "" {
			args = append(args, base)
		}
	}
	if out, err := Run(ctx, repo, args...); err != nil {
		return false, fmt.Errorf("git worktree: %s", strings.TrimSpace(string(out)))
	}
	return created, nil
}

// RemoveWorktree deletes the worktree at dir. Unless force is set, one with
// uncommitted changes is kept and ErrDirtyWorktree is returned.
func RemoveWorktree(ctx context.Context, dir string, force bool) error {
	if !force {
		dirty, err := Dirty(ctx, dir)
		if err != nil {
			return err
		}
		if dirty {
			return ErrDirtyWorktree
		}
	}
	_, err := Run(ctx, dir, "worktree", "remove", "--force", dir)
	return err
}

// IsLinkedWorktree says whether dir, named after id, is a linked worktree: a
// custom location Dory names after the thread, with a .git file where a
// checkout has a folder.
func IsLinkedWorktree(dir, id string) bool {
	st, err := os.Stat(filepath.Join(dir, ".git"))
	return filepath.Base(dir) == id && err == nil && !st.IsDir()
}

// Dirty says whether dir has changes that are not committed.
func Dirty(ctx context.Context, dir string) (bool, error) {
	out, err := Run(ctx, dir, "status", "--porcelain")
	return len(strings.TrimSpace(string(out))) > 0, err
}

// CurrentBranch is the branch dir is on, or "" on a detached HEAD.
func CurrentBranch(ctx context.Context, dir string) (string, error) {
	return Output(ctx, dir, "branch", "--show-current")
}

// Branches is what a new thread can start from in a checkout.
type Branches struct {
	Current  string   `json:"current"`
	Branches []string `json:"branches"`
}

// ListBranches lists the local branches of dir, the latest commit first.
func ListBranches(ctx context.Context, dir string) Branches {
	out := Branches{Branches: []string{}}
	if b, err := CurrentBranch(ctx, dir); err == nil {
		out.Current = b
	}
	if b, err := Output(ctx, dir, "for-each-ref", "--sort=-committerdate", "--format=%(refname:short)", "refs/heads"); err == nil {
		for _, line := range strings.Split(b, "\n") {
			if line != "" {
				out.Branches = append(out.Branches, line)
			}
		}
	}
	return out
}

// SwitchBranch moves a checkout to branch. Git refuses when local changes
// would be lost; its message is returned as is.
func SwitchBranch(ctx context.Context, dir, branch string) error {
	if ListBranches(ctx, dir).Current == branch {
		return nil
	}
	if out, err := Run(ctx, dir, "switch", branch); err != nil {
		return apperr.Newf(apperr.BranchSwitchFailed, "git switch %s failed", branch).With("branch", branch).With("detail", strings.TrimSpace(string(out)))
	}
	return nil
}

// Worktree is a worktree of a repository.
type Worktree struct {
	Path    string `json:"path"`
	Branch  string `json:"branch"`
	Threads int    `json:"threads"`
	Working int    `json:"working"`
}

// Worktrees lists the worktrees of the repository at repo, without the main
// checkout.
func Worktrees(ctx context.Context, repo string) []Worktree {
	out := []Worktree{}
	b, err := Output(ctx, repo, "worktree", "list", "--porcelain")
	if err != nil {
		return out
	}
	for i, block := range strings.Split(b, "\n\n") {
		if i == 0 {
			continue // the main checkout
		}
		var w Worktree
		for _, line := range strings.Split(block, "\n") {
			if p, ok := strings.CutPrefix(line, "worktree "); ok {
				w.Path = p
			} else if br, ok := strings.CutPrefix(line, "branch "); ok {
				w.Branch = strings.TrimPrefix(br, "refs/heads/")
			}
		}
		if w.Path != "" {
			out = append(out, w)
		}
	}
	return out
}

// DefaultBranch is the branch origin points HEAD to, or main.
func DefaultBranch(ctx context.Context, repo string) string {
	if b, err := Output(ctx, repo, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		return strings.TrimPrefix(b, "origin/")
	}
	return "main"
}

// Pull fast-forwards branch in dir from its upstream. It does nothing when dir
// is on another branch, has uncommitted changes or has no upstream.
func Pull(ctx context.Context, dir, branch string) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	text := func(args ...string) (string, error) {
		out, err := Run(ctx, dir, args...)
		return strings.TrimSpace(string(out)), err
	}
	if cur, err := text("rev-parse", "--abbrev-ref", "HEAD"); err != nil || cur != branch {
		return nil
	}
	if _, err := text("rev-parse", "--abbrev-ref", "@{u}"); err != nil {
		return nil
	}
	if dirty, err := Dirty(ctx, dir); err != nil || dirty {
		return err
	}
	if out, err := text("fetch", "--quiet"); err != nil {
		return errors.New(out)
	}
	// Dirty again after the fetch would make the merge refuse; --ff-only never
	// creates a merge commit.
	if out, err := text("merge", "--ff-only", "--quiet", "@{u}"); err != nil {
		return errors.New(out)
	}
	return nil
}
