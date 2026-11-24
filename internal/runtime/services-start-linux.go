//go:build linux

package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/migueel15/dory/internal/harness/kit"
)

// serviceMarker tags every process of a service started by Dory.
const serviceMarker = "DORY_SERVICE=1"

// StartService runs command in the thread's worktree as a service Dory owns:
// in its own session, marked with the thread and detached from the agent, so
// it keeps running after the turn ends. Its output goes to a log file.
func (m *Manager) StartService(ctx context.Context, thread, command string) (pid int, logPath string, err error) {
	if command == "" {
		return 0, "", errors.New("command is required")
	}
	dir, err := m.Dir(ctx, thread)
	if err != nil {
		return 0, "", err
	}
	logs := filepath.Join(os.TempDir(), "dory-services", thread)
	if err := os.MkdirAll(logs, 0o700); err != nil {
		return 0, "", err
	}
	logPath = filepath.Join(logs, fmt.Sprintf("%d.log", time.Now().UnixNano()))
	out, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, "", err
	}
	defer out.Close()
	args := []string{"/bin/sh", "-c", command}
	if scope := kit.UserScope(); scope != "" {
		args = append([]string{scope, "--user", "--scope", "--quiet", "--"}, args...)
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = append(threadEnv(os.Environ(), thread), serviceMarker)
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, "", err
	}
	go cmd.Wait() // reap it if it ends while Dory runs
	return cmd.Process.Pid, logPath, nil
}
