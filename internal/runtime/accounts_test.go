package runtime

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"
)

func TestAccounts(t *testing.T) {
	dir := t.TempDir()
	state := testNodeState(t)
	m := NewManager(nil, nil, "n", dir, state, nil, Claude{Bin: "x"}, Codex{Bin: "x"})
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
	d, _ := m.registry.Get("claude@trabajo")
	if env := d.Accounts.Environ(); env[len(env)-1] != "CLAUDE_CONFIG_DIR="+home {
		t.Fatalf("env %v", env[len(env)-1])
	}
	if err = m.UpdateAccount(ctx, "claude", "Personal", ""); err != nil {
		t.Fatal(err)
	}
	m2 := NewManager(nil, nil, "n", dir, state, nil, Claude{Bin: "x"}, Codex{Bin: "x"})
	base, _ := m2.registry.Get("claude")
	if _, ok := m2.registry.Get("claude@trabajo"); !ok || base.Detect(ctx).Name != "Personal" {
		t.Fatal("accounts not reloaded")
	}
	if err = m2.RemoveAccount("claude"); !errors.Is(err, ErrAccount) {
		t.Fatalf("default must stay, got %v", err)
	}
	if err = m2.RemoveAccount("claude@trabajo"); err != nil {
		t.Fatal("remove failed", err)
	}
	if _, ok := m2.registry.Get("claude@trabajo"); ok {
		t.Fatal("account still registered")
	}
}

func TestThreadEnvKeepsDefaultEnvironment(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	env := threadEnv(nil, "t1")
	if !slices.Contains(env, "PATH=/usr/bin") || !slices.Contains(env, "DORY_THREAD_ID=t1") {
		t.Fatalf("env = %v", env)
	}
}
