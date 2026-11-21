//go:build !windows

package runtime

import (
	"os/exec"
	"syscall"
)

// interruptible runs cmd in its own process group, so an interrupt reaches
// the harness and the tools it started.
func interruptible(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT) }
}

// stopGroup signals the process group of cmd, started by interruptible.
func stopGroup(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, sig)
	}
}
