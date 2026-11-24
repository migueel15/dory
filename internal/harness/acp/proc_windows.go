//go:build windows

package acp

import (
	"os/exec"
	"syscall"
)

// interruptible: Windows has no process groups to signal; the agent is killed.
func interruptible(cmd *exec.Cmd) {}

// stopGroup: Windows has no process groups to signal; the agent is killed.
func stopGroup(cmd *exec.Cmd, _ syscall.Signal) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
