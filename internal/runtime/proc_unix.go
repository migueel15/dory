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
