package kit

import (
	"os"

	"github.com/migueel15/dory/internal/harness"
)

func AccountName(base, label string) string {
	if label == "" {
		return base
	}
	return label
}

// accountEnv is the environment of a harness process; home empty is the default account.
func AccountEnv(key, home string) []string {
	if home == "" {
		return nil
	}
	return append(os.Environ(), key+"="+home)
}

// withEnv sets env on cmd when the account has its own; nil keeps the default.

func AccountDir(p harness.Paths, a interface {
	ID() string
	AccountHome() string
}) string {
	if dir := a.AccountHome(); dir != "" {
		return dir
	}
	return p.Dir(a.ID())
}
