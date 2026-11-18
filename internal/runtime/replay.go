package runtime

import (
	"bytes"
	"context"
	"os"
	"time"
)

// Replay is a test provider: it plays back a recorded Claude Code stream-json
// file, so the interface can be developed without a real agent.
type Replay struct {
	File  string
	Delay time.Duration
}

func (Replay) ID() string { return "replay" }

func (r Replay) Detect(context.Context) Status {
	_, err := os.Stat(r.File)
	return Status{ID: "replay", Name: "Reproducción", Installed: err == nil, Version: "1",
		Models: []Model{{Slug: "grabacion", Name: "Grabación"}}, Capabilities: []string{"interrupt"}}
}

func (r Replay) Run(ctx context.Context, t Turn) (<-chan Event, error) {
	raw, err := os.ReadFile(r.File)
	if err != nil {
		return nil, err
	}
	events := make(chan Event, 64)
	p := &claudeParser{run: t.RunID, texts: map[string]*Message{}, tools: map[string]*Message{}}
	go func() {
		defer close(events)
		done := false
		emit := func(e Event) {
			done = done || e.Done
			events <- e
		}
		for _, line := range bytes.Split(raw, []byte("\n")) {
			select {
			case <-ctx.Done():
				events <- Event{Done: true, Err: "interrumpido"}
				return
			case <-time.After(r.Delay):
			}
			p.parse(line, emit)
		}
		if !done {
			events <- Event{Done: true}
		}
	}()
	return events, nil
}
