package runtime

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestAccounts(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(nil, nil, "n", dir, Claude{Bin: "x"}, Codex{Bin: "x"})
	ctx := context.Background()
	home := filepath.Join(dir, "work")
	a, err := m.AddAccount(ctx, "claude", "Trabajo", home)
	if err != nil || a.ID() != "claude@trabajo" {
		t.Fatalf("add: %v %+v", err, a)
	}
	if _, err = m.AddAccount(ctx, "claude", "Otra", home); !errors.Is(err, ErrAccount) {
		t.Fatalf("same home must fail, got %v", err)
	}
	if _, err = m.AddAccount(ctx, "opencode", "x", home); !errors.Is(err, ErrAccount) {
		t.Fatalf("opencode must fail, got %v", err)
	}
	if env := m.provider0("claude@trabajo").(Claude).Environ(); env[len(env)-1] != "CLAUDE_CONFIG_DIR="+home {
		t.Fatalf("env %v", env[len(env)-1])
	}
	if err = m.UpdateAccount(ctx, "claude", "Personal", ""); err != nil {
		t.Fatal(err)
	}
	m2 := NewManager(nil, nil, "n", dir, Claude{Bin: "x"}, Codex{Bin: "x"})
	if c := m2.provider0("claude@trabajo"); c == nil || m2.provider0("claude").(Claude).Label != "Personal" {
		t.Fatal("accounts not reloaded")
	}
	if err = m2.RemoveAccount("claude"); !errors.Is(err, ErrAccount) {
		t.Fatalf("default must stay, got %v", err)
	}
	if err = m2.RemoveAccount("claude@trabajo"); err != nil || m2.provider0("claude@trabajo") != nil {
		t.Fatal("remove failed", err)
	}
}
