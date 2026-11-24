//go:build !windows

package terminal

import (
	"os"
	"syscall"
	"time"
)

const enter = "\r"

// start runs $SHELL, or /bin/sh, in cwd.
func start(cwd string, env []string, cols, rows int) (PTY, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return launch(shell, nil, cwd, env, cols, rows, killGroup)
}

// killGroup hangs up the process group of the shell, which go-pty makes a
// session of its own, and kills what is left of it a moment later.
func killGroup(p *os.Process) {
	_ = syscall.Kill(-p.Pid, syscall.SIGHUP)
	go func() {
		time.Sleep(500 * time.Millisecond)
		_ = syscall.Kill(-p.Pid, syscall.SIGKILL)
		_ = p.Kill()
	}()
}
