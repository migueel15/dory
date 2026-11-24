package acp

import (
	"context"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/cmdexec"
	"github.com/migueel15/dory/internal/harness"
)

// These helpers are the few the adapter needs from the old runtime package.
// They stay private here until the shared kit of the other adapters exists.

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

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// short keeps the first line of s and at most n runes of it.
func short(s string, n int) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// clip keeps at most n runes of s, all its lines included.
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// mcpName splits "mcp__server__tool".
func mcpName(name string) (server, tool string, ok bool) {
	if !strings.HasPrefix(name, "mcp__") {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(name, "mcp__"), "__", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// lineDiff is a whole-block diff of one replacement: the old lines out, the
// new ones in. It is what the agent reports, not a computed minimal diff.
func lineDiff(path, old, nw string) harness.FileEdit {
	f := harness.FileEdit{Path: path, Change: "update"}
	if old == "" {
		f.Change = "add"
	}
	var b strings.Builder
	write := func(mark string, s string) int {
		if s == "" {
			return 0
		}
		lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
		for _, l := range lines {
			b.WriteString(mark + l + "\n")
		}
		return len(lines)
	}
	f.Removed = write("-", old)
	f.Added = write("+", nw)
	f.Diff = clip(strings.TrimSuffix(b.String(), "\n"), 12000)
	return f
}

// threadEnv marks an agent and its inherited descendants for the owning thread.
// A nil env is the default one: it inherits Dory's own environment.
func threadEnv(env []string, id string) []string {
	if env == nil {
		env = os.Environ()
	}
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, "DORY_THREAD_ID=") {
			out = append(out, entry)
		}
	}
	if id != "" {
		out = append(out, "DORY_THREAD_ID="+id)
	}
	return out
}

var (
	userScopeOnce sync.Once
	userScopePath string
)

// userScope returns systemd-run when Dory runs under systemd and a user
// manager answers, so a turn runs outside the Dory unit's cgroup.
func userScope() string {
	userScopeOnce.Do(func() {
		if runtime.GOOS != "linux" || os.Getenv("INVOCATION_ID") == "" {
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

// agentCommand places a turn outside the Dory service's systemd cgroup when a
// user manager is available. Detached children can then outlive it.
func agentCommand(ctx context.Context, threadID, bin string, args ...string) *exec.Cmd {
	if scope := userScope(); threadID != "" && scope != "" {
		return cmdexec.CommandContext(ctx, scope, append([]string{"--user", "--scope", "--quiet", "--", bin}, args...)...)
	}
	return cmdexec.CommandContext(ctx, bin, args...)
}

// detectVersion runs "<bin> --version" and reports its first line.
func detectVersion(ctx context.Context, bin string) (string, bool) {
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
