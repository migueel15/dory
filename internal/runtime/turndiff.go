package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/migueel15/dory/internal/cmdexec"
)

// A turn's changes come from git, not from the agent's tools: the runtime
// takes a snapshot of the folder when the turn starts and diffs it against
// another when the turn ends, so edits made by any command count.

const maxTurnFiles = 300

// snapshot writes the folder's files, untracked ones included and ignored
// ones left out, as a git tree and returns its id, or "" outside a repository.
// It uses a temporary index, so the user's index and HEAD stay untouched.
func snapshot(ctx context.Context, dir string) string {
	if dir == "" {
		return ""
	}
	if out, err := gitCmd(ctx, dir, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(string(out)) != "true" {
		return ""
	}
	f, err := os.CreateTemp("", "dory-snapshot-*.index")
	if err != nil {
		return ""
	}
	index := f.Name()
	f.Close()
	os.Remove(index) // git creates it; an empty file is not a valid index.
	defer os.Remove(index)
	git := func(args ...string) ([]byte, error) {
		cmd := cmdexec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+index)
		return cmd.Output()
	}
	// Starting from HEAD's index lets git reuse its entries.
	_, _ = git("read-tree", "HEAD")
	if _, err := git("add", "-A", "--", "."); err != nil {
		return ""
	}
	out, err := git("write-tree")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// treeChanges diffs two snapshots of the folder into the files of a turn.
func treeChanges(ctx context.Context, dir, from, to string) []FileEdit {
	if from == "" || to == "" || from == to {
		return nil
	}
	out, err := gitCmd(ctx, dir, "-c", "core.quotepath=false", "diff", "--no-color", "--no-ext-diff", "--no-renames", from, to)
	if err != nil {
		return nil
	}
	return parseGitDiff(string(out))
}

// parseGitDiff splits a unified git diff into its files with their counts.
func parseGitDiff(diff string) []FileEdit {
	var files []FileEdit
	var cur *FileEdit
	var body []string
	flush := func() {
		if cur != nil && len(files) < maxTurnFiles {
			cur.Diff = clip(strings.Join(body, "\n"), 12000)
			files = append(files, *cur)
		}
		cur, body = nil, nil
	}
	hunk := false
	for _, l := range strings.Split(diff, "\n") {
		if strings.HasPrefix(l, "diff --git ") {
			flush()
			cur, hunk = &FileEdit{Change: "update", Path: diffPath(l)}, false
			continue
		}
		if cur == nil {
			continue
		}
		switch {
		case !hunk && strings.HasPrefix(l, "new file mode"):
			cur.Change = "add"
		case !hunk && strings.HasPrefix(l, "deleted file mode"):
			cur.Change = "delete"
		case !hunk && strings.HasPrefix(l, "+++ "), !hunk && strings.HasPrefix(l, "--- "), !hunk && strings.HasPrefix(l, "index "):
		case strings.HasPrefix(l, "@@"):
			hunk = true
			body = append(body, l)
		case hunk && strings.HasPrefix(l, "+"):
			cur.Added++
			body = append(body, l)
		case hunk && strings.HasPrefix(l, "-"):
			cur.Removed++
			body = append(body, l)
		case hunk:
			body = append(body, l)
		}
	}
	flush()
	return files
}

// diffPath reads the path of a "diff --git a/x b/x" line.
func diffPath(l string) string {
	rest := strings.TrimPrefix(l, "diff --git ")
	if i := strings.Index(rest, " b/"); i >= 0 {
		return filepath.ToSlash(rest[i+3:])
	}
	return rest
}
