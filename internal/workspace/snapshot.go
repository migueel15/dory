package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/cmdexec"
	"github.com/migueel15/dory/internal/harness"
)

type FileEdit = harness.FileEdit

// clip keeps at most n runes of s.
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// A turn's changes come from git, not from the agent's tools: the runtime
// takes a snapshot of the folder when the turn starts and diffs it against
// another when the turn ends, so edits made by any command count.

const maxTurnFiles = 300

// The snapshot a turn starts from is kept as refs/dory/turns/<thread>/<run>,
// so git keeps it and the thread can go back to it.
func snapshotRef(thread, run string) string { return "refs/dory/turns/" + thread + "/" + run }

// withIndex runs git in root with a temporary index, so the user's index
// stays untouched.
func withIndex(ctx context.Context, root string, fn func(git func(stdin string, args ...string) ([]byte, error)) error) error {
	f, err := os.CreateTemp("", "dory-snapshot-*.index")
	if err != nil {
		return err
	}
	index := f.Name()
	f.Close()
	os.Remove(index) // git creates it; an empty file is not a valid index.
	defer os.Remove(index)
	return fn(func(stdin string, args ...string) ([]byte, error) {
		cmd := cmdexec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+index)
		if stdin != "" {
			cmd.Stdin = strings.NewReader(stdin)
		}
		return cmd.Output()
	})
}

// Snapshot writes the repository's files, untracked ones included and
// ignored ones left out, as a git tree and returns its id, or "".
func Snapshot(ctx context.Context, root string) string {
	if root == "" {
		return ""
	}
	var tree string
	_ = withIndex(ctx, root, func(git func(string, ...string) ([]byte, error)) error {
		// Starting from HEAD's index lets git reuse its entries.
		_, _ = git("", "read-tree", "HEAD")
		if _, err := git("", "add", "-A"); err != nil {
			return err
		}
		out, err := git("", "write-tree")
		tree = strings.TrimSpace(string(out))
		return err
	})
	return tree
}

// KeepSnapshot stores a turn's starting tree under its ref.
func KeepSnapshot(ctx context.Context, root, thread, run, tree string) {
	if root == "" || tree == "" {
		return
	}
	cmd := cmdexec.CommandContext(ctx, "git", "-C", root, "commit-tree", tree, "-m", "dory: inicio del turno "+run)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Dory", "GIT_AUTHOR_EMAIL=dory@localhost", "GIT_COMMITTER_NAME=Dory", "GIT_COMMITTER_EMAIL=dory@localhost")
	out, err := cmd.Output()
	if err != nil {
		return
	}
	_, _ = Run(ctx, root, "update-ref", snapshotRef(thread, run), strings.TrimSpace(string(out)))
}

type TurnSnapshot struct {
	Ref string
	At  int64 // when the turn started, in Unix seconds
}

// snapshots lists the thread's snapshot refs, oldest first.
func snapshots(ctx context.Context, root, thread string) []TurnSnapshot {
	if root == "" {
		return nil
	}
	out, err := Run(ctx, root, "for-each-ref", "--sort=committerdate", "--format=%(committerdate:unix) %(refname)", snapshotRef(thread, ""))
	if err != nil {
		return nil
	}
	var list []TurnSnapshot
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		at, ref, ok := strings.Cut(l, " ")
		if !ok {
			continue
		}
		n, _ := strconv.ParseInt(at, 10, 64)
		list = append(list, TurnSnapshot{Ref: ref, At: n})
	}
	return list
}

// SnapshotAt finds the snapshot of the turn a user message started: by its
// run, or, when the harness replaced the message with its own and the run is
// lost, by the snapshot taken closest to the message's time.
func SnapshotAt(ctx context.Context, root, thread, run string, at time.Time) (TurnSnapshot, bool) {
	list := snapshots(ctx, root, thread)
	best, gap := TurnSnapshot{}, int64(120)
	for _, s := range list {
		if run != "" && s.Ref == snapshotRef(thread, run) {
			return s, true
		}
		d := s.At - at.Unix()
		if d < 0 {
			d = -d
		}
		if !at.IsZero() && d <= gap {
			best, gap = s, d
		}
	}
	return best, best.Ref != ""
}

// DropSnapshots deletes the thread's snapshot refs taken at or after from,
// or all of them when from is 0.
func DropSnapshots(ctx context.Context, root, thread string, from int64) {
	for _, s := range snapshots(ctx, root, thread) {
		if s.At >= from {
			_, _ = Run(ctx, root, "update-ref", "-d", s.Ref)
		}
	}
}

// RestoreSnapshot puts the repository's files back as they were in a turn's
// snapshot: it rewrites the files that changed, restores deleted ones and
// removes the ones created since.
func RestoreSnapshot(ctx context.Context, root, ref string) error {
	out, err := Run(ctx, root, "rev-parse", "--verify", "--quiet", ref+"^{tree}")
	if err != nil {
		return fmt.Errorf("snapshot %s: %w", ref, err)
	}
	base, now := strings.TrimSpace(string(out)), Snapshot(ctx, root)
	if now == "" {
		return fmt.Errorf("snapshot of %s failed", root)
	}
	added, err := Run(ctx, root, "diff", "--name-only", "-z", "--no-renames", "--diff-filter=A", base, now)
	if err != nil {
		return err
	}
	for _, p := range strings.Split(string(added), "\x00") {
		if p != "" {
			_ = os.Remove(filepath.Join(root, filepath.FromSlash(p)))
		}
	}
	changed, err := Run(ctx, root, "diff", "--name-only", "-z", "--no-renames", "--diff-filter=MDT", base, now)
	if err != nil || len(changed) == 0 {
		return err
	}
	return withIndex(ctx, root, func(git func(string, ...string) ([]byte, error)) error {
		if _, err := git("", "read-tree", base); err != nil {
			return err
		}
		_, err := git(string(changed), "checkout-index", "-f", "-z", "--stdin")
		return err
	})
}

// TreeChanges diffs two snapshots of the folder into the files of a turn.
func TreeChanges(ctx context.Context, dir, from, to string) []FileEdit {
	if from == "" || to == "" || from == to {
		return nil
	}
	out, err := Run(ctx, dir, "-c", "core.quotepath=false", "diff", "--no-color", "--no-ext-diff", "--no-renames", from, to)
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
