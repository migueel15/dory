//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// detach runs the core on its own, without a console window and in its own
// process group, so closing the app or the terminal that started it does
// not stop it.
func detach(cmd *exec.Cmd) {
	const createNoWindow, detachedProcess, newProcessGroup = 0x08000000, 0x00000008, 0x00000200
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow | detachedProcess | newProcessGroup, HideWindow: true}
}

func terminate(p *os.Process) error { return p.Kill() }
