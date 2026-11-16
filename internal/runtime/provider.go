// Package runtime runs agent harnesses (Claude Code, Codex…) as child
// processes and turns their output into one normalized stream of messages.
// Each harness has an adapter; the rest of Dory never sees harness formats.
package runtime

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"strings"
	"time"
)

// Message is one entry of a conversation as the interface shows it. Role is
// user, assistant, reasoning, tool, plan or system. An adapter sends the same
// ID again to update a message while it streams.
type Message struct {
	ID        string `json:"id"`
	ThreadID  string `json:"thread_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Role      string `json:"role"`
	Text      string `json:"text"`
	Title     string `json:"title,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Status    string `json:"status,omitempty"`
	Streaming bool   `json:"streaming"`
	CreatedAt string `json:"created_at"`
}

// Window is how much of one usage window of a provider is used, 0 to 1.
type Window struct {
	Kind        string    `json:"kind"`
	Utilization float64   `json:"utilization"`
	ResetsAt    time.Time `json:"resets_at"`
}

// Event is what an adapter reports while a turn runs. Exactly one of its
// fields is set.
type Event struct {
	Message *Message
	// Session is the harness id of the conversation, used to resume it.
	Session string
	Usage   []Window
	// Done ends the turn; Err is set when it failed.
	Done bool
	Err  string
}

// Turn is one message of the user and everything the agent does after it.
type Turn struct {
	RunID   string
	Dir     string
	Text    string
	Model   string
	Effort  string
	Session string
	// FullAccess skips the harness approval prompts.
	FullAccess bool
	// MCPURL and MCPToken point the agent at the Dory MCP of this node.
	MCPURL, MCPToken string
}

// Model is one model a provider offers, with its reasoning effort levels.
type Model struct {
	Slug    string   `json:"slug"`
	Name    string   `json:"name"`
	Efforts []string `json:"efforts,omitempty"`
}

// Status says whether a harness is installed on this machine.
type Status struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Installed bool    `json:"installed"`
	Version   string  `json:"version"`
	Models    []Model `json:"models"`
	// Capabilities lists what the interface may offer for this provider.
	Capabilities []string `json:"capabilities"`
	LoggedIn     bool     `json:"logged_in"`
	Account      string   `json:"account,omitempty"`
}

// Provider is the adapter of one harness.
type Provider interface {
	ID() string
	Detect(ctx context.Context) Status
	// Run starts a turn and streams its events until a Done event; the
	// channel is closed afterwards. Cancelling ctx interrupts the turn.
	Run(ctx context.Context, t Turn) (<-chan Event, error)
}

// detect runs "<bin> --version" and reports its first line.
func detect(ctx context.Context, bin string) (string, bool) {
	path, err := exec.LookPath(bin)
	if err != nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return "", true
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return line, true
}

// stream starts cmd and feeds each stdout line to parse, which turns it into
// events. Cancelling ctx interrupts the process gently, then kills it.
func stream(ctx context.Context, cmd *exec.Cmd, stdin string, parse func(line []byte, emit func(Event))) (<-chan Event, error) {
	interruptible(cmd)
	cmd.WaitDelay = 10 * time.Second
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
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
