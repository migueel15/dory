package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// A harness can be signed in with several accounts on one node, as long as
// each one has its own configuration directory (CLAUDE_CONFIG_DIR,
// CODEX_HOME). The default account keeps the plain id ("claude"); an extra one
// is "claude@work". Accounts belong to the node and are not replicated.

// ErrAccount reports an account that cannot be created or changed.
var ErrAccount = errors.New("runtime: invalid account")

// Account is the saved configuration of one account of a harness. An empty
// Slug is the default account, whose directory is the harness default unless
// Home says otherwise.
type Account struct {
	Driver string `json:"driver"`
	Slug   string `json:"slug,omitempty"`
	Name   string `json:"name,omitempty"`
	Home   string `json:"home,omitempty"`
}

// ID is the provider id of the account.
func (a Account) ID() string { return accountID(a.Driver, a.Slug) }

func accountID(driver, slug string) string {
	if slug == "" {
		return driver
	}
	return driver + "@" + slug
}

// driverOf is the harness of a provider id.
func driverOf(id string) string {
	d, _, _ := strings.Cut(id, "@")
	return d
}

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

func (m *Manager) accountsFile() string { return filepath.Join(m.dataDir, "accounts.json") }

func (m *Manager) loadAccounts() {
	raw, err := os.ReadFile(m.accountsFile())
	if m.dataDir == "" || err != nil {
		return
	}
	var saved []Account
	if json.Unmarshal(raw, &saved) != nil {
		return
	}
	for _, a := range saved {
		m.accounts = append(m.accounts, a)
		if p := buildAccount(a, m.providers[a.Driver]); p != nil {
			m.providers[a.ID()] = p
		}
	}
}

// buildAccount makes the provider of an account from the default one, which
// supplies the binary.
func buildAccount(a Account, base Provider) Provider {
	switch b := base.(type) {
	case Claude:
		b.Account, b.Home, b.Label = a.Slug, a.Home, a.Name
		return b
	case Codex:
		b.Account, b.Home, b.Label = a.Slug, a.Home, a.Name
		return b
	}
	return nil
}

func (m *Manager) saveAccounts() error {
	if m.dataDir == "" {
		return nil
	}
	raw, _ := json.MarshalIndent(m.accounts, "", "  ")
	tmp := m.accountsFile() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.accountsFile())
}

// provider0 returns a registered provider, turned on or not.
func (m *Manager) provider0(id string) Provider {
	m.provMu.RLock()
	defer m.provMu.RUnlock()
	return m.providers[id]
}

// registered is a snapshot of the providers by id.
func (m *Manager) registered() map[string]Provider {
	m.provMu.RLock()
	defer m.provMu.RUnlock()
	out := make(map[string]Provider, len(m.providers))
	for id, p := range m.providers {
		out[id] = p
	}
	return out
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// defaultHome is where a harness keeps its configuration without an account.
func defaultHome(driver string) string {
	home, _ := os.UserHomeDir()
	if driver == "claude" {
		return filepath.Join(home, ".claude")
	}
	return filepath.Join(home, ".codex")
}

func cleanHome(home string) (string, error) {
	home = strings.TrimSpace(home)
	if strings.HasPrefix(home, "~/") {
		h, _ := os.UserHomeDir()
		home = filepath.Join(h, home[2:])
	}
	if home != "" && !filepath.IsAbs(home) {
		return "", fmt.Errorf("%w: la carpeta debe ser una ruta absoluta", ErrAccount)
	}
	return filepath.Clean(home), nil
}

// homeTaken says whether another account of the driver already uses home.
// The default account counts with its harness default.
func (m *Manager) homeTaken(driver, home, except string) bool {
	for id, p := range m.providers {
		if id == except || driverOf(id) != driver {
			continue
		}
		var h string
		switch a := p.(type) {
		case Claude:
			h = a.Home
		case Codex:
			h = a.Home
		}
		if h == "" {
			h = defaultHome(driver)
		}
		if h == home {
			return true
		}
	}
	return false
}

// AddAccount registers another account of a harness. Without a directory it
// gets one under the data directory.
func (m *Manager) AddAccount(ctx context.Context, driver, name, home string) (Account, error) {
	name = strings.TrimSpace(name)
	if driver != "claude" && driver != "codex" {
		return Account{}, fmt.Errorf("%w: %s no admite varias cuentas", ErrAccount, driver)
	}
	if name == "" {
		return Account{}, fmt.Errorf("%w: falta el nombre", ErrAccount)
	}
	home, err := cleanHome(home)
	if err != nil {
		return Account{}, err
	}
	m.provMu.Lock()
	defer m.provMu.Unlock()
	base := slugRe.ReplaceAllString(strings.ToLower(name), "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "cuenta"
	}
	slug := base
	for n := 2; m.providers[accountID(driver, slug)] != nil; n++ {
		slug = fmt.Sprintf("%s-%d", base, n)
	}
	if home == "." || home == "" {
		home = filepath.Join(m.dataDir, "accounts", driver+"-"+slug)
	}
	if m.homeTaken(driver, home, "") {
		return Account{}, fmt.Errorf("%w: otra cuenta ya usa esa carpeta", ErrAccount)
	}
	a := Account{Driver: driver, Slug: slug, Name: name, Home: home}
	p := buildAccount(a, m.providers[driver])
	if p == nil {
		return Account{}, ErrProvider
	}
	if err = os.MkdirAll(home, 0o700); err != nil {
		return Account{}, err
	}
	m.providers[a.ID()] = p
	m.accounts = append(m.accounts, a)
	if err = m.saveAccounts(); err != nil {
		return Account{}, err
	}
	m.invalidate()
	return a, nil
}

// UpdateAccount changes the display name and directory of an account, the
// default one included. An empty home on the default account restores the
// harness default.
func (m *Manager) UpdateAccount(ctx context.Context, id, name, home string) error {
	home, err := cleanHome(home)
	if err != nil {
		return err
	}
	if home == "." {
		home = ""
	}
	m.provMu.Lock()
	defer m.provMu.Unlock()
	cur, ok := m.providers[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrProvider, id)
	}
	driver := driverOf(id)
	if home == "" && id != driver {
		return fmt.Errorf("%w: falta la carpeta", ErrAccount)
	}
	if home != "" && m.homeTaken(driver, home, id) {
		return fmt.Errorf("%w: otra cuenta ya usa esa carpeta", ErrAccount)
	}
	_, slug, _ := strings.Cut(id, "@")
	a := Account{Driver: driver, Slug: slug, Name: strings.TrimSpace(name), Home: home}
	p := buildAccount(a, cur)
	if p == nil {
		return ErrProvider
	}
	if home != "" {
		if err = os.MkdirAll(home, 0o700); err != nil {
			return err
		}
	}
	m.providers[id] = p
	found := false
	for i := range m.accounts {
		if m.accounts[i].ID() == id {
			m.accounts[i], found = a, true
		}
	}
	if !found {
		m.accounts = append(m.accounts, a)
	}
	if err = m.saveAccounts(); err != nil {
		return err
	}
	m.invalidate()
	return nil
}

// RemoveAccount forgets an extra account. Its directory and credentials stay on disk.
func (m *Manager) RemoveAccount(id string) error {
	if id == driverOf(id) {
		return fmt.Errorf("%w: la cuenta predeterminada no se elimina", ErrAccount)
	}
	m.provMu.Lock()
	defer m.provMu.Unlock()
	if m.providers[id] == nil {
		return fmt.Errorf("%w: %s", ErrProvider, id)
	}
	delete(m.providers, id)
	kept := m.accounts[:0]
	for _, a := range m.accounts {
		if a.ID() != id {
			kept = append(kept, a)
		}
	}
	m.accounts = kept
	if err := m.saveAccounts(); err != nil {
		return err
	}
	m.invalidate()
	return nil
}
