// Package workspace holds everything Dory asks of git: worktrees, branches,
// diffs, pulls and the snapshots of a folder taken around each turn. It has no
// dependency on the runtime, the api or the database.
package workspace

import (
	"context"
	"strings"

	"github.com/migueel15/dory/internal/cmdexec"
)

// Run runs git in dir and returns its combined output.
func Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return cmdexec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).CombinedOutput()
}

// Output runs git in dir and returns its standard output, trimmed.
func Output(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := cmdexec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}

// ValidBranchName says whether git accepts name as a branch name.
func ValidBranchName(name string) bool {
	return cmdexec.CommandContext(context.Background(), "git", "check-ref-format", "--branch", name).Run() == nil
}

// Files lists the files of the repository at dir that git tracks or that are
// not ignored.
func Files(ctx context.Context, dir string) ([]string, error) {
	out, err := Output(ctx, dir, "ls-files", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}
