//go:build windows

package runtime

import (
	"os/exec"
	"syscall"
)

// interruptible: Windows has no process groups to signal; the harness is killed.
func interruptible(cmd *exec.Cmd) {}

// stopGroup: Windows has no process groups to signal; the harness is killed.
func stopGroup(cmd *exec.Cmd, _ syscall.Signal) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
