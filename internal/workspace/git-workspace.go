package workspace

import "context"

// Git exposes the workspace operations used to prepare a thread.
type Git struct{}

func (Git) Worktrees(ctx context.Context, repo string) []Worktree {
	return Worktrees(ctx, repo)
}

func (Git) DefaultBranch(ctx context.Context, repo string) string {
	return DefaultBranch(ctx, repo)
}

func (Git) BranchExists(ctx context.Context, repo, branch string) bool {
	return BranchExists(ctx, repo, branch)
}

func (Git) SwitchBranch(ctx context.Context, repo, branch string) error {
	return SwitchBranch(ctx, repo, branch)
}

func (Git) AddWorktree(ctx context.Context, repo, dir, branch, base string) (bool, error) {
	return addWorktree(ctx, repo, dir, branch, base)
}

func (Git) RemoveWorktree(ctx context.Context, dir string, force bool) error {
	return RemoveWorktree(ctx, dir, force)
}

func (Git) DeleteBranch(ctx context.Context, repo, branch string) error {
	_, err := Run(ctx, repo, "branch", "-D", "--", branch)
	return err
}

func (Git) Pull(ctx context.Context, dir, branch string) error {
	return Pull(ctx, dir, branch)
}
