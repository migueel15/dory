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
		s.events <- harness.RequestOpened{Request: Request{ID: "req-1", Kind: "question", Questions: []Question{{Question: "¿Color?"}}}}
		select {
		case a := <-s.answers:
			reply := "(sin respuesta)"
			if len(a) > 0 && len(a[0]) > 0 {
				reply = a[0][0]
			}
			s.events <- harness.TextDone{ItemID: "m1", Kind: harness.Assistant, Text: reply, Meta: harness.ItemMeta{RunID: t.RunID}}
			s.events <- harness.TurnEnded{Reason: harness.Completed}
		case <-ctx.Done():
		}
	}()
	return adapt(s.Events(), s.Answer), nil
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

func TestManagerAllowsEmptyAnswer(t *testing.T) {
	ctx := context.Background()
	m, store := manager(t, 0)
	m.registry.Register(asker{})
	th, _ := registerTestThread(ctx, m, data.Thread{Provider: "asker"})
	if err := m.Send(ctx, th.ID, "pregunta"); err != nil {
		t.Fatal(err)
	}
	wait(t, store, th.ID, "waiting")
	if err := m.Answer(ctx, th.ID, "req-1", [][]string{{}}); err != nil {
		t.Fatalf("empty answer failed: %v", err)
	}
	wait(t, store, th.ID, "idle")
	if reqs, _ := m.Requests(ctx, th.ID); len(reqs) != 0 {
		t.Fatalf("requests left: %+v", reqs)
	}
}
