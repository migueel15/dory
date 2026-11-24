package runtime

import (
	"errors"
	"testing"

	"github.com/migueel15/dory/internal/data"
)

func TestRegisterRollsBackThreadWhenLocalInsertFails(t *testing.T) {
	m, store := manager(t, 0)
	ctx := t.Context()
	if _, err := store.LocalDB().ExecContext(ctx, `CREATE TRIGGER deny_runtime BEFORE INSERT ON thread_runtime
		BEGIN SELECT RAISE(ABORT, 'runtime insert failed'); END`); err != nil {
		t.Fatal(err)
	}
	thread := data.Thread{ID: data.NewThreadID(), NodeID: m.node, Provider: "replay"}
	if _, err := m.Register(ctx, thread, t.TempDir(), ""); err == nil {
		t.Fatal("registration succeeded")
	}
	if _, err := store.Threads().Get(ctx, thread.ID); !errors.Is(err, data.ErrNotFound) {
		t.Fatalf("thread exists after failed registration: %v", err)
	}
	if _, err := m.Dir(ctx, thread.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("runtime exists after failed registration: %v", err)
	}
}
