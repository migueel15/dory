//go:build windows

package terminal

import (
	"os"
	"os/exec"
	"strconv"
)

const enter = "\r"

// start runs pwsh, or Windows PowerShell, or %COMSPEC%, in cwd on a ConPTY.
func start(cwd string, env []string, cols, rows int) (PTY, error) {
	shell := ""
	for _, name := range []string{"pwsh.exe", "powershell.exe"} {
		if path, err := exec.LookPath(name); err == nil {
			shell = path
			break
		}
	}
	if shell == "" {
		shell = os.Getenv("COMSPEC")
	}
	if shell == "" {
		shell = "cmd.exe"
	}
	return launch(shell, nil, cwd, env, cols, rows, killTree)
}

// killTree ends the shell and every process it started.
func killTree(p *os.Process) {
	_ = exec.Command("taskkill", "/PID", strconv.Itoa(p.Pid), "/T", "/F").Run()
	_ = p.Kill()
}
