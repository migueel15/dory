package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/oplog"
)

// Register persists a prepared thread and its local runtime.
func (m *Manager) Register(ctx context.Context, t data.Thread, dir, baseBranch string) (data.Thread, error) {
	if dir != "" {
		return m.register(ctx, t, dir, baseBranch, "")
	}
	dir = filepath.Join(m.dataDir, "scratch", t.ID)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return data.Thread{}, err
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return data.Thread{}, err
	}
	out, err := m.register(ctx, t, dir, baseBranch, "")
	if err != nil {
		err = errors.Join(err, os.Remove(dir))
	}
	return out, err
}

func (m *Manager) register(ctx context.Context, t data.Thread, dir, baseBranch, session string) (data.Thread, error) {
	return m.store.CreateLocalThread(ctx, t, func(tx *oplog.Tx, saved data.Thread) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO thread_runtime(thread_id, worktree_path, base_branch, session_provider, session_id) VALUES (?, ?, ?, ?, ?)`,
			saved.ID, dir, baseBranch, saved.Provider, session)
		return err
	})
}

// RegisterExternal resumes an existing session and copies its history into Dory.
func (m *Manager) RegisterExternal(ctx context.Context, t data.Thread, e External) (data.Thread, error) {
	m.adoptMu.Lock()
	defer m.adoptMu.Unlock()
	adopted, err := m.Adopted(ctx)
	if err != nil {
		return data.Thread{}, err
	}
	if id := adopted[e.ID]; id != "" {
		return m.store.Threads().Get(ctx, id)
	}
	if _, ok := m.provider(e.Provider); !ok {
		return data.Thread{}, ErrProvider.Msgf("%s", e.Provider)
	}
	msgs, err := m.ReadExternal(ctx, e)
	if err != nil {
		return data.Thread{}, err
	}
	t, err = m.register(ctx, t, e.Dir, "", e.ID)
	if err != nil {
		return t, err
	}
	for _, msg := range msgs {
		msg.ThreadID = t.ID
		msg.ID = t.ID + ":" + msg.ID
		m.save(ctx, msg)
	}
	return t, nil
}

func (m *Manager) CloseThread(id string) {
	m.CloseLive(id)
	StopThreadServices(id)
}
