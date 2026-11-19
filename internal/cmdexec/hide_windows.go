//go:build windows

package cmdexec

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// hide starts the command without a console window.
func hide(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}
}
