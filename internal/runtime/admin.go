package runtime

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/cmdexec"
)

// Admin is what a harness offers to manage it from Dory: its session, its
// login flow and its update. Credentials stay inside the harness.
type Admin interface {
	// Session reports whether the harness is signed in, and how.
	Session(ctx context.Context) (bool, string)
	LoginCommand() []string
	UpdateCommand() []string
	InstallCommand() []string
}

func (c Claude) Session(ctx context.Context) (bool, string) {
	out, err := withEnv(cmdexec.CommandContext(ctx, c.bin(), "auth", "status"), c.Environ()).Output()
	var st struct {
		LoggedIn   bool   `json:"loggedIn"`
		AuthMethod string `json:"authMethod"`
	}
	if err != nil || json.Unmarshal(out, &st) != nil {
		return false, ""
	}
	return st.LoggedIn, st.AuthMethod
}
func (c Claude) LoginCommand() []string  { return []string{c.bin(), "auth", "login"} }
func (c Claude) UpdateCommand() []string { return []string{c.bin(), "update"} }

func (c Codex) Session(ctx context.Context) (bool, string) {
	out, err := withEnv(cmdexec.CommandContext(ctx, c.bin(), "login", "status"), c.Environ()).CombinedOutput()
	text := strings.TrimSpace(string(out))
	return err == nil && strings.HasPrefix(text, "Logged in"), text
}
func (c Codex) LoginCommand() []string  { return []string{c.bin(), "login", "--device-auth"} }
func (c Codex) UpdateCommand() []string { return []string{c.bin(), "update"} }

func (o OpenCode) Session(ctx context.Context) (bool, string) {
	out, err := cmdexec.CommandContext(ctx, o.bin(), "auth", "list").Output()
	lines := 0
	for _, l := range strings.Split(string(out), "\n") {
		if strings.Contains(l, "stored") {
			lines++
		}
	}
	return err == nil && lines > 0, strings.TrimSpace(string(out))
}
func (o OpenCode) LoginCommand() []string  { return []string{o.bin(), "auth", "login"} }
func (o OpenCode) UpdateCommand() []string { return []string{o.bin(), "upgrade"} }

// Job is a harness command started from the interface, such as a login that
// prints a URL and a code and waits for the browser.
type Job struct {
	ID          string    `json:"id"`
	Provider    string    `json:"provider"`
	Kind        string    `json:"kind"`
	Output      string    `json:"output"`
	Done        bool      `json:"done"`
	Error       string    `json:"error,omitempty"`
	Started     time.Time `json:"started"`
	AcceptsCode bool      `json:"accepts_code"`
	input       *os.File
}

type jobs struct {
	mu   sync.Mutex
	list map[string]*Job
}

var running = &jobs{list: map[string]*Job{}}

type jobOutput struct {
	j *Job
}

func (w jobOutput) Write(p []byte) (int, error) {
	running.mu.Lock()
	defer running.mu.Unlock()
	if len(w.j.Output) < 64<<10 {
		w.j.Output += string(p)
	}
	return len(p), nil
}

// StartJob runs the login, update or install command of a provider in the background.
func (m *Manager) StartJob(provider, kind string) (*Job, error) {
	p, ok := m.provider0(provider).(Admin)
	if !ok {
		return nil, ErrProvider
	}
	var args []string
	switch kind {
	case "login":
		args = p.LoginCommand()
	case "update":
		args = p.UpdateCommand()
	case "install":
		args = p.InstallCommand()
	default:
		return nil, errors.New("unknown job " + kind)
	}
	j := &Job{ID: strings.ToLower(rand.Text()), Provider: provider, Kind: kind, Started: time.Now()}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	cmd := cmdexec.CommandContext(ctx, args[0], args[1:]...)
	if env, ok := m.provider0(provider).(interface{ Environ() []string }); ok {
		cmd.Env = env.Environ()
	}
	if d := driverOf(provider); (d == "claude" || d == antigravityID) && kind == "login" {
		reader, writer, err := os.Pipe()
		if err != nil {
			cancel()
			return nil, err
		}
		defer reader.Close()
		cmd.Stdin = reader
		j.input = writer
		j.AcceptsCode = true
	}
	cmd.Stdout, cmd.Stderr = jobOutput{j}, jobOutput{j}
	if err := cmd.Start(); err != nil {
		if j.input != nil {
			j.input.Close()
		}
		cancel()
		return nil, err
	}
	running.mu.Lock()
	running.list[j.ID] = j
	snapshot := *j
	running.mu.Unlock()
	go func() {
		defer cancel()
		err := cmd.Wait()
		running.mu.Lock()
		j.Done = true
		j.AcceptsCode = false
		if j.input != nil {
			j.input.Close()
			j.input = nil
		}
		if err != nil {
			j.Error = err.Error()
		}
		running.mu.Unlock()
		m.invalidate()
	}()
	return &snapshot, nil
}

var ErrJobInputClosed = errors.New("el intento ya no admite códigos")

// SubmitLoginCode writes a single authorization code to the active CLI.
func (m *Manager) SubmitLoginCode(id, code string) error {
	running.mu.Lock()
	defer running.mu.Unlock()
	j, ok := running.list[id]
	if !ok {
		return ErrNotFound
	}
	if !j.AcceptsCode || j.input == nil {
		return ErrJobInputClosed
	}
	j.AcceptsCode = false
	if _, err := j.input.WriteString(code + "\n"); err != nil {
		return ErrJobInputClosed
	}
	return nil
}

// JobState returns a copy of a job.
func (m *Manager) JobState(id string) (Job, bool) {
	running.mu.Lock()
	defer running.mu.Unlock()
	j, ok := running.list[id]
	if !ok {
		return Job{}, false
	}
	return *j, true
}

// UpdateAll runs the update command of every installed harness, one after
// another, and returns the error of each one that failed.
func (m *Manager) UpdateAll(ctx context.Context) map[string]string {
	failed := map[string]string{}
	for _, st := range m.Providers(ctx) {
		p, ok := m.provider0(st.ID).(Admin)
		if !ok || !st.Installed {
			continue
		}
		args := p.UpdateCommand()
		run, cancel := context.WithTimeout(ctx, 10*time.Minute)
		upd := cmdexec.CommandContext(run, args[0], args[1:]...)
		if env, ok := p.(interface{ Environ() []string }); ok {
			upd.Env = env.Environ()
		}
		out, err := upd.CombinedOutput()
		cancel()
		if err != nil {
			failed[st.ID] = strings.TrimSpace(string(out))
		}
	}
	return failed
}
