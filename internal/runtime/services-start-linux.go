//go:build linux

package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

var (
	userScopeOnce sync.Once
	userScopePath string
)

// userScope returns systemd-run when Dory runs under systemd and a user
// manager answers, so a service gets its own scope and outlives Dory's unit.
func userScope() string {
	userScopeOnce.Do(func() {
		if os.Getenv("INVOCATION_ID") == "" {
			return
		}
		path, err := exec.LookPath("systemd-run")
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if exec.CommandContext(ctx, "systemctl", "--user", "show-environment").Run() == nil {
			userScopePath = path
		}
	})
	return userScopePath
}

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
	if scope := userScope(); scope != "" {
		args = append([]string{scope, "--user", "--scope", "--quiet", "--"}, args...)
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = threadEnv(os.Environ(), thread)
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, "", err
	}
	go cmd.Wait() // reap it if it ends while Dory runs
	return cmd.Process.Pid, logPath, nil
}
