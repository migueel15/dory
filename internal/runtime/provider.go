// Package runtime runs agent harnesses (Claude Code, Codex…) as child
// processes and turns their output into one normalized stream of messages.
// Each harness has an adapter; the rest of Dory never sees harness formats.
package runtime

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/cmdexec"
	"github.com/migueel15/dory/internal/harness"
)

type Message = harness.Message

type Window = harness.Window

type ReportedSettings = harness.ReportedSettings

type Event = harness.LegacyEvent

type Turn = harness.Turn

// promptWithInstructions is the fallback for a harness that cannot take
// session instructions: they precede only the first prompt of a new session,
// which the harness then keeps in its own history.
func promptWithInstructions(t Turn) string {
	if t.Instructions == "" || t.Session != "" {
		return t.Text
	}
	return "<dory_instructions>\n" + t.Instructions + "\n</dory_instructions>\n\n" + t.Text
}

// threadEnv marks a provider and its inherited descendants for the owning thread.
// A nil env is the default account's: it inherits Dory's own environment.
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

// providerCommand places a turn outside the Dory service's systemd cgroup
// when a user manager is available. Detached children can then outlive it.
func providerCommand(ctx context.Context, threadID, bin string, args ...string) *exec.Cmd {
	if scope := userScope(); threadID != "" && scope != "" {
		return cmdexec.CommandContext(ctx, scope, append([]string{"--user", "--scope", "--quiet", "--", bin}, args...)...)
	}
	return cmdexec.CommandContext(ctx, bin, args...)
}

type Model = harness.Model

type Status = harness.Status

type Provider = harness.Driver

// detect runs "<bin> --version" and reports its first line.
func detect(ctx context.Context, bin string) (string, bool) {
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

// stream starts cmd and feeds each stdout line to parse, which turns it into
// events. Cancelling ctx interrupts the process gently, then kills it.
func stream(ctx context.Context, cmd *exec.Cmd, stdin string, parse func(line []byte, emit func(Event))) (<-chan Event, error) {
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	return pipe(ctx, cmd, parse)
}

// pipe starts cmd, whose stdin the caller has set up, and parses its stdout
// as stream does.
func pipe(ctx context.Context, cmd *exec.Cmd, parse func(line []byte, emit func(Event))) (<-chan Event, error) {
	interruptible(cmd)
	cmd.WaitDelay = 10 * time.Second
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr strings.Builder
	cmd.Stderr = &limited{w: &stderr, n: 8 << 10}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	events := make(chan Event, 64)
	go func() {
		defer close(events)
		done := false
		emit := func(e Event) {
			done = done || e.Done
			events <- e
		}
		scanner := bufio.NewScanner(out)
		scanner.Buffer(make([]byte, 64<<10), 32<<20)
		for scanner.Scan() {
			parse(scanner.Bytes(), emit)
		}
		err := cmd.Wait()
		if done {
			return
		}
		msg := strings.TrimSpace(stderr.String())
		switch {
		case ctx.Err() != nil:
			msg = "interrumpido"
		case err != nil && msg == "":
			msg = err.Error()
		case msg == "":
			msg = "el agente terminó sin respuesta"
		}
		events <- Event{Done: true, Err: msg}
	}()
	return events, nil
}

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
