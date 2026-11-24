package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/harness"
)

// asker is an interactive provider whose turn asks one question and waits.
type asker struct{}

func (asker) Info() harness.Info {
	return harness.Info{ID: "asker", Capabilities: harness.Capabilities{MidTurnRequests: true}}
}
func (asker) Detect(context.Context) Status           { return Status{ID: "asker", Installed: true} }
func (asker) Usage(context.Context) ([]Window, error) { return nil, ErrNoUsage }
func (asker) Start(ctx context.Context, t Turn) (Session, error) {
	s := &askSession{events: make(chan Event, 8), answers: make(chan [][]string, 1)}
	go func() {
		defer close(s.events)
		s.events <- Event{Request: &Request{ID: "req-1", Kind: "question", Questions: []Question{{Question: "¿Color?"}}}}
		select {
		case a := <-s.answers:
			s.events <- Event{Message: &Message{ID: "m1", RunID: t.RunID, Role: "assistant", Text: a[0][0]}}
			s.events <- Event{Done: true}
		case <-ctx.Done():
		}
	}()
	return harness.Adapt(s.Events(), s.Answer), nil
}

type askSession struct {
	events  chan Event
	answers chan [][]string
}

func (s *askSession) Events() <-chan Event { return s.events }
func (s *askSession) Answer(id string, a [][]string) error {
	if id != "req-1" {
		return ErrNoRequest
	}
	s.answers <- a
	return nil
}

func TestManagerWaitsForAnswer(t *testing.T) {
	ctx := context.Background()
	m, store := manager(t, 0)
	m.registry.Register(asker{})
	th, _ := registerTestThread(ctx, m, data.Thread{Provider: "asker"})
	if err := m.Send(ctx, th.ID, "pregunta"); err != nil {
		t.Fatal(err)
	}
	wait(t, store, th.ID, "waiting")
	reqs, err := m.Requests(ctx, th.ID)
	if err != nil || len(reqs) != 1 || reqs[0].Questions[0].Question != "¿Color?" {
		t.Fatalf("requests: %+v %v", reqs, err)
	}
	if err = m.Answer(ctx, th.ID, "otra", nil); !errors.Is(err, ErrNoRequest) {
		t.Fatalf("unknown request: %v", err)
	}
	if err = m.Answer(ctx, th.ID, "req-1", [][]string{{"Azul"}}); err != nil {
		t.Fatal(err)
	}
	wait(t, store, th.ID, "idle")
	msgs, _ := m.Messages(ctx, th.ID)
	users := 0
	for _, msg := range msgs {
		if msg.Role == "user" {
			users++
		}
	}
	if last := msgs[len(msgs)-1]; last.Text != "Azul" || users != 1 {
		t.Fatalf("messages: %+v", msgs)
	}
	if reqs, _ = m.Requests(ctx, th.ID); len(reqs) != 0 {
		t.Fatalf("requests left: %+v", reqs)
	}
}
