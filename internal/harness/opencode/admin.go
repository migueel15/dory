package opencode

import (
	"context"
	"runtime"
	"strings"

	"github.com/migueel15/dory/internal/cmdexec"
	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
)

func (o OpenCode) Session(ctx context.Context) (bool, string) {
	out, err := cmdexec.CommandContext(ctx, o.bin(), "auth", "list").Output()
	lines := 0
	for _, l := range strings.Split(string(out), "\n") {
		if strings.Contains(l, "stored") {
			lines++
		}
	}
	return err == nil && lines > 0, strings.TrimSpace(string(out))
}
func (o OpenCode) LoginCommand() []string  { return []string{o.bin(), "auth", "login"} }
func (o OpenCode) UpdateCommand() []string { return []string{o.bin(), "upgrade"} }

// OpenCode publishes no installer for Windows; npm is its documented route.
func (OpenCode) InstallCommand() []string {
	if runtime.GOOS == "windows" {
		return []string{"npm", "install", "-g", "@opencode/cli@latest"}
	}
	return kit.ShellInstall("https://opencode.ai/v2/install", "")
}

func (OpenCode) Usage(context.Context) ([]harness.Window, error) { return nil, harness.ErrNoUsage }
