package runtime

import (
	"bytes"
	"context"
	"os"
	"time"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/claude"
)

// Replay is a test provider: it plays back a recorded Claude Code stream-json
// file, so the runtime can be tested without a real agent.
type Replay struct {
	File  string
	Delay time.Duration
}

func (Replay) Info() harness.Info {
	return harness.Info{ID: "replay", Driver: "replay", Name: "Reproducción"}
}

func (Replay) Usage(context.Context) ([]Window, error) { return nil, ErrNoUsage }

func (r Replay) Detect(context.Context) Status {
	_, err := os.Stat(r.File)
	return Status{ID: "replay", Name: "Reproducción", Installed: err == nil, Version: "1",
		Models: []Model{{Slug: "grabacion", Name: "Grabación"}}, Capabilities: []string{"interrupt"}}
}

func (r Replay) Start(ctx context.Context, t Turn) (Session, error) {
	raw, err := os.ReadFile(r.File)
	if err != nil {
		return nil, err
	}
	events := make(chan harness.Event, 64)
	parse := claude.LineParser(t.RunID)
	go func() {
		defer close(events)
		done := false
		emit := func(e harness.Event) {
			_, end := e.(harness.TurnEnded)
			done = done || end
			events <- e
		}
		for _, line := range bytes.Split(raw, []byte("\n")) {
			select {
			case <-ctx.Done():
				events <- harness.TurnEnded{Reason: harness.Interrupted}
				return
			case <-time.After(r.Delay):
			}
			parse(line, emit)
		}
		if !done {
			events <- harness.TurnEnded{Reason: harness.Completed}
		}
	}()
	return replaySession{events}, nil
}

type replaySession struct{ events <-chan harness.Event }

func (s replaySession) Events() <-chan harness.Event   { return s.events }
func (replaySession) Respond(string, [][]string) error { return ErrNoRequest }
func rejectResponse(string, [][]string) error          { return ErrNoRequest }
