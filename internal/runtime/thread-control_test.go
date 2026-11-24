package runtime

import (
	"context"
	"errors"
	"github.com/migueel15/dory/internal/harness"
	"testing"

	"github.com/migueel15/dory/internal/data"
)

func TestConfigureRunningThreadAppliesAfterTurn(t *testing.T) {
	ctx := context.Background()
	m, store := manager(t, 0)
	claude := capturedProvider{"claude", make(chan Turn, 1), make(chan chan Event, 1)}
	codex := capturedProvider{"codex", make(chan Turn, 1), make(chan chan Event, 1)}
	m.registry.Register(claude)
	m.registry.Register(codex)
	thread, err := registerTestThread(ctx, m, data.Thread{Provider: claude.id, Model: "opus"})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Send(ctx, thread.ID, "diseña"); err != nil {
		t.Fatal(err)
	}
	<-claude.turns
	events := <-claude.ends

	provider, model := codex.id, "gpt-6.2-sol"
	_, applied, err := m.ConfigureThread(ctx, thread.ID, ThreadSettings{Provider: &provider, Model: &model})
	if err != nil || applied != AppliedAfterTurn {
		t.Fatalf("applied=%q err=%v", applied, err)
	}
	if th, _ := store.Threads().Get(ctx, thread.ID); th.Provider != claude.id {
		t.Fatalf("changed during the turn: %+v", th)
	}
	if err = m.Send(ctx, thread.ID, "ejecuta"); err != nil {
		t.Fatal(err)
	}
	events <- harness.TurnEnded{Reason: harness.Completed}
	close(events)

	next := <-codex.turns
	if next.Model != model {
		t.Fatalf("next turn model %q", next.Model)
	}
	if m.Pending(thread.ID) != nil {
		t.Fatal("pending settings kept")
	}
	close(<-codex.ends)
	if th := wait(t, store, thread.ID, "idle"); th.Provider != codex.id || th.Model != model {
		t.Fatalf("thread %+v", th)
	}
}

func TestConfigureIdleThreadAndResolveNames(t *testing.T) {
	ctx := context.Background()
	m, _ := manager(t, 0)
	thread, err := registerTestThread(ctx, m, data.Thread{Provider: "replay", Model: "grabacion"})
	if err != nil {
		t.Fatal(err)
	}
	name := "GRABACIÓN"
	th, applied, err := m.ConfigureThread(ctx, thread.ID, ThreadSettings{Model: &name})
	if err != nil || applied != AppliedNow || th.Model != "grabacion" {
		t.Fatalf("thread=%+v applied=%q err=%v", th, applied, err)
	}
	bad := "nope"
	if _, _, err = m.ConfigureThread(ctx, thread.ID, ThreadSettings{Model: &bad}); !errors.Is(err, data.ErrInvalid) {
		t.Fatalf("unknown model accepted: %v", err)
	}
	mode := "full-access"
	if _, _, err = m.ConfigureThread(ctx, thread.ID, ThreadSettings{RuntimeMode: &mode}); !errors.Is(err, data.ErrInvalid) {
		t.Fatalf("runtime mode changed: %v", err)
	}
}
