//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// detach runs the core on its own, without a console window.
func detach(cmd *exec.Cmd) {
	const createNoWindow, detachedProcess = 0x08000000, 0x00000008
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow | detachedProcess, HideWindow: true}
}

func terminate(p *os.Process) error { return p.Kill() }
