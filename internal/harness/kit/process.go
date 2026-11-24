package kit

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/cmdexec"
	"github.com/migueel15/dory/internal/harness"
)

// detect runs "<bin> --version" and reports its first line.
func Detect(ctx context.Context, bin string) (string, bool) {
	path, err := exec.LookPath(bin)
	if err != nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := cmdexec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return "", true
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return line, true
}

func LimitWriter(w io.Writer, n int) io.Writer { return &limited{w: w, n: n} }

type limited struct {
	w io.Writer
	n int
}

func (l *limited) Write(p []byte) (int, error) {
	if l.n > 0 {
		k := min(len(p), l.n)
		l.n -= k
		l.w.Write(p[:k])
	}
	return len(p), nil
}

func Now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// short keeps the first line of s and at most n runes of it.
func Short(s string, n int) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// clip keeps at most n runes of s, all its lines included.
func Clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func WithEnv(cmd *exec.Cmd, env []string) *exec.Cmd {
	if env != nil {
		cmd.Env = env
	}
	return cmd
}

// ProcessEnd reports why a process or output stream ended without a terminal event.
func ProcessEnd(ctxErr, err error, stderr string) harness.TurnEnded {
	if ctxErr != nil {
		return harness.TurnEnded{Reason: harness.Interrupted}
	}
	if stderr != "" {
		return harness.TurnEnded{Reason: harness.Failed, Error: stderr}
	}
	if err != nil {
		return harness.TurnEnded{Reason: harness.Failed, Error: err.Error()}
	}
	return harness.TurnEnded{Reason: harness.NoResponse}
}
