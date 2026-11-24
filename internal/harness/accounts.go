package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/migueel15/dory/internal/apperr"
)

// ErrAccount reports an account that cannot be created or changed.
var ErrAccount = apperr.New(apperr.InvalidAccount, "invalid account")

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
func (a Account) ID() string { return AccountID(a.Driver, a.Slug) }

func AccountID(driver, slug string) string {
	if slug == "" {
		return driver
	}
	return driver + "@" + slug
}

// driverOf is the harness of a provider id.
func DriverOf(id string) string {
	d, _, _ := strings.Cut(id, "@")
	return d
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func cleanHome(home string) (string, error) {
	home = strings.TrimSpace(home)
	if strings.HasPrefix(home, "~/") {
		h, _ := os.UserHomeDir()
		home = filepath.Join(h, home[2:])
	}
	if home != "" && !filepath.IsAbs(home) {
		return "", ErrAccount.Msgf("the folder must be an absolute path").With("reason", "folder_not_absolute")
	}
	return filepath.Clean(home), nil
}

// homeTaken says whether another account of the driver already uses home.
// The default account counts with its harness default.
func (r *Registry) homeTaken(driver, home, except string) bool {
	for id, p := range r.providers {
		if id == except || DriverOf(id) != driver {
			continue
		}
		if p.Accounts == nil {
			continue
		}
		h := p.Accounts.AccountHome()
		if h == "" {
			userHome, _ := os.UserHomeDir()
			h = p.Accounts.DefaultHome(userHome)
		}
		if h == home {
			return true
		}
	}
	return false
}

// AddAccount registers another account of a harness. Without a directory it
// gets one under the data directory.
func (r *Registry) AddAccount(ctx context.Context, driver, name, home string) (Account, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Account{}, ErrAccount.Msgf("the name is missing").With("reason", "name_missing")
	}
	home, err := cleanHome(home)
	if err != nil {
		return Account{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.providers[driver].Accounts == nil {
		return Account{}, ErrAccount.Msgf("%s does not support several accounts", driver).With("reason", "multiple_accounts_unsupported").With("driver", driver)
	}
	base := slugRe.ReplaceAllString(strings.ToLower(name), "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "cuenta"
	}
	slug := base
	for n := 2; r.providers[AccountID(driver, slug)].Driver != nil; n++ {
		slug = fmt.Sprintf("%s-%d", base, n)
	}
	if home == "." || home == "" {
		home = filepath.Join(r.dataDir, "accounts", driver+"-"+slug)
	}
	if r.homeTaken(driver, home, "") {
		return Account{}, ErrAccount.Msgf("another account already uses that folder").With("reason", "folder_in_use")
	}
	a := Account{Driver: driver, Slug: slug, Name: name, Home: home}
	p := entry(r.providers[driver].Accounts.WithAccount(a))
	if err = os.MkdirAll(home, 0o700); err != nil {
		return Account{}, err
	}
	accounts := append(slices.Clone(r.accounts.Get()), a)
	if err = r.accounts.Set(ctx, accounts); err != nil {
		return Account{}, err
	}
	r.providers[a.ID()] = p
	return a, nil
}

// UpdateAccount changes the display name and directory of an account, the
// default one included. An empty home on the default account restores the
// harness default.
func (r *Registry) UpdateAccount(ctx context.Context, id, name, home string) error {
	home, err := cleanHome(home)
	if err != nil {
		return err
	}
	if home == "." {
		home = ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.providers[id]
	if !ok {
		return ErrProvider.Msgf("%s", id)
	}
	driver := DriverOf(id)
	if home == "" && id != driver {
		return ErrAccount.Msgf("the folder is missing").With("reason", "folder_missing")
	}
	if home != "" && r.homeTaken(driver, home, id) {
		return ErrAccount.Msgf("another account already uses that folder").With("reason", "folder_in_use")
	}
	_, slug, _ := strings.Cut(id, "@")
	a := Account{Driver: driver, Slug: slug, Name: strings.TrimSpace(name), Home: home}
	if cur.Accounts == nil {
		return ErrProvider
	}
	p := entry(cur.Accounts.WithAccount(a))
	if home != "" {
		if err = os.MkdirAll(home, 0o700); err != nil {
			return err
		}
	}
	accounts := slices.Clone(r.accounts.Get())
	i := slices.IndexFunc(accounts, func(a Account) bool { return a.ID() == id })
	if i < 0 {
		accounts = append(accounts, a)
	} else {
		accounts[i] = a
	}
	if err = r.accounts.Set(ctx, accounts); err != nil {
		return err
	}
	r.providers[id] = p
	return nil
}

// RemoveAccount forgets an extra account. Its directory and credentials stay on disk.
func (r *Registry) RemoveAccount(id string) error {
	if id == DriverOf(id) {
		return ErrAccount.Msgf("the default account cannot be removed").With("reason", "default_account_locked")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.providers[id].Driver == nil {
		return ErrProvider.Msgf("%s", id)
	}
	accounts := slices.DeleteFunc(slices.Clone(r.accounts.Get()), func(a Account) bool { return a.ID() == id })
	if err := r.accounts.Set(context.Background(), accounts); err != nil {
		return err
	}
	delete(r.providers, id)
	return nil
}
