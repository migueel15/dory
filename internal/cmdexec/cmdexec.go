// Package cmdexec builds the commands Dory runs in the background. On Windows
// they start without a console window, so harnesses, git and helper tools do
// not flash terminals on screen.
package cmdexec

import (
	"context"
	"os/exec"
)

// Command is exec.Command for a background command.
func Command(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)
	hide(cmd)
	return cmd
}

// CommandContext is exec.CommandContext for a background command.
func CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, arg...)
	hide(cmd)
	return cmd
}
