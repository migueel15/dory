//go:build !windows

package cmdexec

import "os/exec"

// hide: only Windows opens console windows for child processes.
func hide(*exec.Cmd) {}
