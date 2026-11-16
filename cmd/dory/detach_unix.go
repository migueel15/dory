//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// detach lets a started process outlive this one.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

func terminate(p *os.Process) error { return p.Signal(syscall.SIGTERM) }
