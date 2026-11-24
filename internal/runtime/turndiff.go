package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/migueel15/dory/internal/cmdexec"
)

// A turn's changes come from git, not from the agent's tools: the runtime
// takes a snapshot of the folder when the turn starts and diffs it against
// another when the turn ends, so edits made by any command count.

const maxTurnFiles = 300

// The snapshot a turn starts from is kept as refs/dory/turns/<thread>/<run>,
// so git keeps it and the thread can go back to it.
func snapshotRef(thread, run string) string { return "refs/dory/turns/" + thread + "/" + run }

// repoRoot returns the top folder of the repository dir is in, or "".
func repoRoot(ctx context.Context, dir string) string {
	if dir == "" {
		return ""
	}
	out, err := gitCmd(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

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

// snapshot writes the repository's files, untracked ones included and
// ignored ones left out, as a git tree and returns its id, or "".
func snapshot(ctx context.Context, root string) string {
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

// keepSnapshot stores a turn's starting tree under its ref.
func keepSnapshot(ctx context.Context, root, thread, run, tree string) {
	if root == "" || tree == "" {
		return
	}
	cmd := cmdexec.CommandContext(ctx, "git", "-C", root, "commit-tree", tree, "-m", "dory: inicio del turno "+run)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Dory", "GIT_AUTHOR_EMAIL=dory@localhost", "GIT_COMMITTER_NAME=Dory", "GIT_COMMITTER_EMAIL=dory@localhost")
	out, err := cmd.Output()
	if err != nil {
		return
	}
	_, _ = gitCmd(ctx, root, "update-ref", snapshotRef(thread, run), strings.TrimSpace(string(out)))
}

// dropSnapshots deletes the thread's snapshot refs whose run keep rejects.
func dropSnapshots(ctx context.Context, root, thread string, keep func(run string) bool) {
	if root == "" {
		return
	}
	prefix := snapshotRef(thread, "")
	out, err := gitCmd(ctx, root, "for-each-ref", "--format=%(refname)", prefix)
	if err != nil {
		return
	}
	for _, ref := range strings.Fields(string(out)) {
		if keep == nil || !keep(strings.TrimPrefix(ref, prefix)) {
			_, _ = gitCmd(ctx, root, "update-ref", "-d", ref)
		}
	}
}

// restoreSnapshot puts the repository's files back as they were in a turn's
// snapshot: it rewrites the files that changed, restores deleted ones and
// removes the ones created since. It reports false when there is no snapshot.
func restoreSnapshot(ctx context.Context, root, thread, run string) (bool, error) {
	if root == "" {
		return false, nil
	}
	out, err := gitCmd(ctx, root, "rev-parse", "--verify", "--quiet", snapshotRef(thread, run)+"^{tree}")
	if err != nil {
		return false, nil
	}
	base, now := strings.TrimSpace(string(out)), snapshot(ctx, root)
	if now == "" {
		return false, fmt.Errorf("snapshot of %s failed", root)
	}
	added, err := gitCmd(ctx, root, "diff", "--name-only", "-z", "--no-renames", "--diff-filter=A", base, now)
	if err != nil {
		return false, err
	}
	for _, p := range strings.Split(string(added), "\x00") {
		if p != "" {
			_ = os.Remove(filepath.Join(root, filepath.FromSlash(p)))
		}
	}
	changed, err := gitCmd(ctx, root, "diff", "--name-only", "-z", "--no-renames", "--diff-filter=MDT", base, now)
	if err != nil || len(changed) == 0 {
		return true, err
	}
	return true, withIndex(ctx, root, func(git func(string, ...string) ([]byte, error)) error {
		if _, err := git("", "read-tree", base); err != nil {
			return err
		}
		_, err := git(string(changed), "checkout-index", "-f", "-z", "--stdin")
		return err
	})
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

// Reverted is what going back to a message left: its text for the composer
// and whether the files went back too.
type Reverted struct {
	Text          string `json:"text"`
	FilesRestored bool   `json:"files_restored"`
}

// Revert takes the thread back to before one of its user messages: the files
// return to the snapshot of that turn, the message and everything after it
// leave the conversation, and the agent starts its next turn from the
// conversation that remains.
func (m *Manager) Revert(ctx context.Context, id, messageID string) (Reverted, error) {
	if _, err := m.local(ctx, id); err != nil {
		return Reverted{}, err
	}
	if m.Running(id) {
		return Reverted{}, ErrBusy
	}
	msgs, err := m.Messages(ctx, id)
	if err != nil {
		return Reverted{}, err
	}
	at := -1
	for i, msg := range msgs {
		if msg.ID == messageID && msg.Role == "user" {
			at = i
			break
		}
	}
	if at < 0 {
		return Reverted{}, ErrNotFound
	}
	target := msgs[at]
	dir, err := m.Dir(ctx, id)
	if err != nil {
		return Reverted{}, err
	}
	root := repoRoot(ctx, dir)
	var out Reverted
	if target.RunID != "" {
		if out.FilesRestored, err = restoreSnapshot(ctx, root, id, target.RunID); err != nil {
			return Reverted{}, err
		}
	}
	if _, err := m.db.ExecContext(ctx, `DELETE FROM thread_messages WHERE thread_id = ? AND seq >= (SELECT seq FROM thread_messages WHERE id = ?)`, id, messageID); err != nil {
		return Reverted{}, err
	}
	// No session holds the shortened conversation: the next turn starts a new
	// one with the history that remains, as after a change of provider.
	if _, err := m.db.ExecContext(ctx, `UPDATE thread_runtime SET session_id = '', session_provider = '' WHERE thread_id = ?`, id); err != nil {
		return Reverted{}, err
	}
	CloseLive(id)
	runs := map[string]bool{}
	for _, msg := range msgs[:at] {
		runs[msg.RunID] = true
	}
	dropSnapshots(ctx, root, id, func(run string) bool { return runs[run] })
	m.publish(Update{ThreadID: id, Reload: true})
	out.Text = target.Text
	return out, nil
}

// DropSnapshots deletes every turn snapshot of a thread.
func (m *Manager) DropSnapshots(ctx context.Context, id string) {
	if dir, err := m.Dir(ctx, id); err == nil {
		dropSnapshots(ctx, repoRoot(ctx, dir), id, nil)
	}
}
