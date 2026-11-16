//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// detach lets a started process outlive this one.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
