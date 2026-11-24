package runtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/migueel15/dory/internal/harness"
)

var ErrAccount = harness.ErrAccount

type Account = harness.Account
type Paths = harness.Paths

func accountID(driver, slug string) string { return harness.AccountID(driver, slug) }
func driverOf(id string) string            { return harness.DriverOf(id) }
func accountName(base, label string) string {
	if label == "" {
		return base
	}
	return label
}

// accountEnv is the environment of a harness process; home empty is the default account.
func accountEnv(key, home string) []string {
	if home == "" {
		return nil
	}
	return append(os.Environ(), key+"="+home)
}

// withEnv sets env on cmd when the account has its own; nil keeps the default.
func withEnv(cmd *exec.Cmd, env []string) *exec.Cmd {
	if env != nil {
		cmd.Env = env
	}
	return cmd
}

func (m *Manager) Paths(home string) Paths {
	return Paths{Home: home, Config: func(driver string) string {
		if p, ok := m.registry.Get(driver); ok && p.Accounts != nil {
			if dir := p.Accounts.AccountHome(); dir != "" {
				return dir
			}
			return p.Accounts.DefaultHome(home)
		}
		return ""
	}}
}

func (m *Manager) AddAccount(ctx context.Context, driver, name, home string) (Account, error) {
	a, err := m.registry.AddAccount(ctx, driver, name, home)
	if err == nil {
		m.invalidate()
	}
	return a, err
}
func (m *Manager) UpdateAccount(ctx context.Context, id, name, home string) error {
	if err := m.registry.UpdateAccount(ctx, id, name, home); err != nil {
		return err
	}
	m.invalidate()
	return nil
}
func (m *Manager) RemoveAccount(id string) error {
	if err := m.registry.RemoveAccount(id); err != nil {
		return err
	}
	m.invalidate()
	return nil
}

// Defaults are the folders this node uses when a setting is left empty.
type Defaults struct {
	Worktrees  string `json:"worktrees"`
	Accounts   string `json:"accounts"`
	Home       string `json:"home"`
	ClaudeHome string `json:"claude_home"`
	CodexHome  string `json:"codex_home"`
}

func (m *Manager) Defaults() Defaults {
	home, _ := os.UserHomeDir()
	return Defaults{
		Worktrees: filepath.Join(m.dataDir, "worktrees"), Accounts: filepath.Join(m.dataDir, "accounts"), Home: home,
		ClaudeHome: (Claude{}).DefaultHome(home), CodexHome: (Codex{}).DefaultHome(home),
	}
}

func accountDir(p Paths, a interface {
	ID() string
	AccountHome() string
}) string {
	if dir := a.AccountHome(); dir != "" {
		return dir
	}
	return p.Dir(a.ID())
}
