package runtime

import (
	"context"
	"time"

	"github.com/migueel15/dory/internal/workspace"
)

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
	root := workspace.RepoRoot(ctx, dir)
	var out Reverted
	sent, _ := time.Parse(time.RFC3339Nano, target.CreatedAt)
	snap, ok := workspace.SnapshotAt(ctx, root, id, target.RunID, sent)
	if ok {
		if err := workspace.RestoreSnapshot(ctx, root, snap.Ref); err != nil {
			return Reverted{}, err
		}
		out.FilesRestored = true
	}
	if _, err := m.db.ExecContext(ctx, `DELETE FROM thread_messages WHERE thread_id = ? AND seq >= (SELECT seq FROM thread_messages WHERE id = ?)`, id, messageID); err != nil {
		return Reverted{}, err
	}
	// No session holds the shortened conversation: the next turn starts a new
	// one with the history that remains, as after a change of provider.
	if _, err := m.db.ExecContext(ctx, `UPDATE thread_runtime SET session_id = '', session_provider = '' WHERE thread_id = ?`, id); err != nil {
		return Reverted{}, err
	}
	m.CloseLive(id)
	if ok {
		workspace.DropSnapshots(ctx, root, id, snap.At)
	} else if !sent.IsZero() {
		workspace.DropSnapshots(ctx, root, id, sent.Unix())
	}
	m.publish(Update{ThreadID: id, Reload: true})
	out.Text = target.Text
	return out, nil
}

// DropSnapshots deletes every turn snapshot of a thread.
func (m *Manager) DropSnapshots(ctx context.Context, id string) {
	if dir, err := m.Dir(ctx, id); err == nil {
		workspace.DropSnapshots(ctx, workspace.RepoRoot(ctx, dir), id, 0)
	}
}
