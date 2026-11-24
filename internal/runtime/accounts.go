package runtime

import (
	"context"
	"os"
	"path/filepath"

	"github.com/migueel15/dory/internal/harness"
)

type Account = harness.Account
type Paths = harness.Paths

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
	Worktrees string `json:"worktrees"`
	Accounts  string `json:"accounts"`
	Home      string `json:"home"`
}

func (m *Manager) Defaults() Defaults {
	home, _ := os.UserHomeDir()
	return Defaults{
		Worktrees: filepath.Join(m.dataDir, "worktrees"), Accounts: filepath.Join(m.dataDir, "accounts"), Home: home,
	}
}
