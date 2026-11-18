//go:build windows

package runtime

import "os/exec"

// interruptible: Windows has no process groups to signal; the harness is killed.
func interruptible(cmd *exec.Cmd) {}
