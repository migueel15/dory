package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/harness"
)

func scriptedProvider(m *Manager, turns int) capturedProvider {
	p := capturedProvider{"claude", make(chan Turn, turns), make(chan chan Event, turns)}
	m.registry.Register(p)
	return p
}

func end(events chan Event) {
	events <- harness.TurnEnded{Reason: harness.Completed}
	close(events)
}

func TestRunnerQueuesAndDrainsInOrder(t *testing.T) {
	ctx := context.Background()
	m, store := manager(t, 0)
	p := scriptedProvider(m, 3)
	th, _ := registerTestThread(ctx, m, data.Thread{Provider: p.id, Model: "haiku"})
	for _, text := range []string{"uno", "dos", "tres"} {
		if err := m.Send(ctx, th.ID, text); err != nil {
			t.Fatal(err)
		}
	}
	if q := m.Queue(th.ID); len(q) != 2 || q[0].Text != "dos" {
		t.Fatalf("queue %+v", q)
	}
	if !m.Running(th.ID) || m.Active() != 1 {
		t.Fatal("not running")
	}
	if err := m.Clear(ctx, th.ID); !errors.Is(err, ErrBusy) {
		t.Fatalf("clear while running: %v", err)
	}
	for _, want := range []string{"uno", "dos", "tres"} {
		turn := <-p.turns
		if turn.Text != want {
			t.Fatalf("turn %q, want %q", turn.Text, want)
		}
		end(<-p.ends)
	}
	settled(t, m, th.ID)
	wait(t, store, th.ID, "idle")
	if q := m.Queue(th.ID); len(q) != 0 || m.Running(th.ID) {
		t.Fatalf("left queue %+v", q)
	}
}

func TestRunnerSteerRunsChosenMessageAndInterruptDropsQueue(t *testing.T) {
	ctx := context.Background()
	m, _ := manager(t, 0)
	p := scriptedProvider(m, 3)
	th, _ := registerTestThread(ctx, m, data.Thread{Provider: p.id, Model: "haiku"})
	m.Send(ctx, th.ID, "uno")
	<-p.turns
	first := <-p.ends
	m.Send(ctx, th.ID, "dos")
	m.Send(ctx, th.ID, "tres")
	q := m.Queue(th.ID)
	if err := m.SteerQueued(ctx, th.ID, q[1].RunID); err != nil {
		t.Fatal(err)
	}
	if err := m.SteerQueued(ctx, th.ID, "nada"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown queued: %v", err)
	}
	// The harness ends the stopped turn.
	first <- harness.TurnEnded{Reason: harness.Interrupted}
	close(first)
	if turn := <-p.turns; turn.Text != "tres" {
		t.Fatalf("steered turn %q", turn.Text)
	}
	second := <-p.ends
	if q := m.Queue(th.ID); len(q) != 1 || q[0].Text != "dos" {
		t.Fatalf("queue %+v", q)
	}
	if err := m.Interrupt(ctx, th.ID); err != nil {
		t.Fatal(err)
	}
	if len(m.Queue(th.ID)) != 0 {
		t.Fatal("interrupt kept the queue")
	}
	second <- harness.TurnEnded{Reason: harness.Interrupted}
	close(second)
	settled(t, m, th.ID)
	if m.Running(th.ID) {
		t.Fatal("still running")
	}
}

func TestRunnerKeepsQueueOverRestart(t *testing.T) {
	ctx := context.Background()
	m, store := manager(t, 0)
	p := scriptedProvider(m, 2)
	th, _ := registerTestThread(ctx, m, data.Thread{Provider: p.id, Model: "haiku"})
	m.Send(ctx, th.ID, "uno")
	<-p.turns
	<-p.ends
	m.Send(ctx, th.ID, "dos")

	// A new manager over the same database is a restarted node.
	m2 := NewManager(m.db, store, m.node, m.dataDir, m.state, nil)
	p2 := scriptedProvider(m2, 2)
	if err := m2.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if q := m2.Queue(th.ID); len(q) != 1 || q[0].Text != "dos" {
		t.Fatalf("queue after restart %+v", q)
	}
	if m2.Running(th.ID) {
		t.Fatal("queue ran on its own")
	}
	if err := m2.SteerQueued(ctx, th.ID, m2.Queue(th.ID)[0].RunID); err != nil {
		t.Fatal(err)
	}
	if turn := <-p2.turns; turn.Text != "dos" {
		t.Fatalf("turn %q", turn.Text)
	}
	end(<-p2.ends)
	settled(t, m2, th.ID)
	if q := m2.Queue(th.ID); len(q) != 0 {
		t.Fatalf("queue %+v", q)
	}
}

func TestRunnerEndsWhenIdle(t *testing.T) {
	ctx := context.Background()
	m, _ := manager(t, 0)
	m.runnerIdle = 20 * time.Millisecond
	th, _ := registerTestThread(ctx, m, data.Thread{Provider: "replay", Model: "grabacion"})
	if err := m.Interrupt(ctx, th.ID); err != nil {
		t.Fatal(err)
	}
	for range 100 {
		if m.lookup(th.ID) == nil {
			// A later command starts a new actor.
			if err := m.Interrupt(ctx, th.ID); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("idle runner did not end")
}

func TestRunnerWaitsForInputWhileAsking(t *testing.T) {
	ctx := context.Background()
	m, store := manager(t, 0)
	m.registry.Register(asker{})
	th, _ := registerTestThread(ctx, m, data.Thread{Provider: "asker"})
	m.Send(ctx, th.ID, "pregunta")
	wait(t, store, th.ID, "waiting")
	for range 100 {
		if m.view(th.ID).State == stateWaitingInput {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if m.view(th.ID).State != stateWaitingInput {
		t.Fatalf("state %v", m.view(th.ID).State)
	}
	if err := m.Answer(ctx, th.ID, "req-1", [][]string{{"Azul"}}); err != nil {
		t.Fatal(err)
	}
	wait(t, store, th.ID, "idle")
	if err := m.Answer(ctx, th.ID, "req-1", nil); !errors.Is(err, ErrNoRequest) {
		t.Fatalf("answer after the turn: %v", err)
	}
}

// settled waits until the thread has no turn.
func settled(t *testing.T, m *Manager, id string) {
	t.Helper()
	for range 300 {
		if !m.Running(id) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("turn never ended")
}

// slowProvider holds the start of a turn until gate is closed.
type slowProvider struct {
	capturedProvider
	gate chan struct{}
}

func (p slowProvider) Start(ctx context.Context, turn Turn) (Session, error) {
	<-p.gate
	return p.capturedProvider.Start(ctx, turn)
}

// The harness starts outside the actor: an interrupt does not wait for it,
// and the turn it stopped ends as soon as the harness is up.
func TestRunnerInterruptDoesNotWaitForStart(t *testing.T) {
	ctx := context.Background()
	m, _ := manager(t, 0)
	p := slowProvider{capturedProvider{"claude", make(chan Turn, 1), make(chan chan Event, 1)}, make(chan struct{})}
	m.registry.Register(p)
	th, _ := registerTestThread(ctx, m, data.Thread{Provider: p.id, Model: "haiku"})
	sent := make(chan error, 1)
	go func() { sent <- m.Send(ctx, th.ID, "uno") }()
	for !m.Running(th.ID) {
		time.Sleep(5 * time.Millisecond)
	}
	interrupted := make(chan error, 1)
	go func() { interrupted <- m.Interrupt(ctx, th.ID) }()
	select {
	case err := <-interrupted:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("interrupt waited for the harness to start")
	}
	close(p.gate)
	if err := <-sent; err != nil {
		t.Fatalf("send = %v", err)
	}
	<-p.turns
	events := <-p.ends
	events <- harness.TurnEnded{Reason: harness.Interrupted}
	close(events)
	settled(t, m, th.ID)
}
