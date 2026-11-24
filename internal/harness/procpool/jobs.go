package procpool

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/cmdexec"
)

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

type jobOutput struct {
	pool *Pool
	j    *Job
}

func (w jobOutput) Write(p []byte) (int, error) {
	w.pool.jobsMu.Lock()
	defer w.pool.jobsMu.Unlock()
	if len(w.j.Output) < 64<<10 {
		w.j.Output += string(p)
	}
	return len(p), nil
}

// StartJob runs the login, update or install command of a provider in the background.
func (p *Pool) StartJob(provider, kind string, args, env []string, acceptsCode bool, finished func()) (*Job, error) {
	if len(args) == 0 {
		return nil, errors.New("missing command for " + kind)
	}
	j := &Job{ID: strings.ToLower(rand.Text()), Provider: provider, Kind: kind, Started: time.Now()}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	cmd := cmdexec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = env
	if acceptsCode {
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
	cmd.Stdout, cmd.Stderr = jobOutput{p, j}, jobOutput{p, j}
	if err := cmd.Start(); err != nil {
		if j.input != nil {
			j.input.Close()
		}
		cancel()
		return nil, err
	}
	p.jobsMu.Lock()
	p.jobs[j.ID] = j
	snapshot := *j
	p.jobsMu.Unlock()
	go func() {
		defer cancel()
		err := cmd.Wait()
		p.jobsMu.Lock()
		j.Done = true
		j.AcceptsCode = false
		if j.input != nil {
			j.input.Close()
			j.input = nil
		}
		if err != nil {
			j.Error = err.Error()
		}
		p.jobsMu.Unlock()
		if finished != nil {
			finished()
		}
	}()
	return &snapshot, nil
}

var ErrJobInputClosed = apperr.New(apperr.JobInputClosed, "the login attempt no longer accepts codes")

// SubmitLoginCode writes a single authorization code to the active CLI.
func (p *Pool) SubmitLoginCode(id, code string) error {
	p.jobsMu.Lock()
	defer p.jobsMu.Unlock()
	j, ok := p.jobs[id]
	if !ok {
		return apperr.New(apperr.NotFound, "job not found")
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
func (p *Pool) JobState(id string) (Job, bool) {
	p.jobsMu.Lock()
	defer p.jobsMu.Unlock()
	j, ok := p.jobs[id]
	if !ok {
		return Job{}, false
	}
	return *j, true
}
