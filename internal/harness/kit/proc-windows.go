//go:build windows

package kit

import (
	"os/exec"
	"syscall"
)

// Interruptible: Windows has no process groups to signal; the harness is killed.
func Interruptible(cmd *exec.Cmd) {}

// StopGroup: Windows has no process groups to signal; the harness is killed.
func StopGroup(cmd *exec.Cmd, _ syscall.Signal) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
