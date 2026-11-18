//go:build windows

package main

import (
	"os"
	"os/exec"
)

func detach(cmd *exec.Cmd) {}

func terminate(p *os.Process) error { return p.Kill() }
