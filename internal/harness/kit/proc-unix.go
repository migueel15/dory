//go:build !windows

package kit

import (
	"os/exec"
	"syscall"
)

// Interruptible runs cmd in its own process group, so an interrupt reaches
// the harness and the tools it started.
func Interruptible(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT) }
}

// StopGroup signals the process group of cmd, started by Interruptible.
func StopGroup(cmd *exec.Cmd, sig syscall.Signal) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, sig)
	}
}
