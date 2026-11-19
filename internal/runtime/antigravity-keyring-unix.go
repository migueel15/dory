//go:build !windows

package runtime

import (
	"context"
	"os/exec"
	"runtime"
	"time"
)

func readAgyKeyring(ctx context.Context) []byte {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", "gemini", "-a", "antigravity", "-w")
	case "linux":
		cmd = exec.CommandContext(ctx, "secret-tool", "lookup", "service", "gemini", "username", "antigravity")
	default:
		return nil
	}
	raw, err := cmd.Output()
	if err != nil && runtime.GOOS == "linux" {
		raw, err = exec.CommandContext(ctx, "secret-tool", "lookup", "service", "gemini", "account", "antigravity").Output()
	}
	if err != nil || len(raw) > antigravitySecretLimit {
		return nil
	}
	return raw
}
