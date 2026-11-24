//go:build linux

package kit

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"time"
)

var (
	userScopeOnce sync.Once
	userScopePath string
)

// userScope returns systemd-run when Dory runs under systemd and a user
// manager answers, so a service gets its own scope and outlives Dory's unit.
func UserScope() string {
	userScopeOnce.Do(func() {
		if os.Getenv("INVOCATION_ID") == "" {
			return
		}
		path, err := exec.LookPath("systemd-run")
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if exec.CommandContext(ctx, "systemctl", "--user", "show-environment").Run() == nil {
			userScopePath = path
		}
	})
	return userScopePath
}
