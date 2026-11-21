package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/data"
)

func TestSpawnRunsChildAndWakesParent(t *testing.T) {
	ctx := context.Background()
	m, store := manager(t, 0)
	claude := capturedProvider{"claude", make(chan Turn, 2), make(chan chan Event, 2)}
	codex := capturedProvider{"codex", make(chan Turn, 1), make(chan chan Event, 1)}
	m.providers[claude.id], m.providers[codex.id] = claude, codex
	parent, err := m.Create(ctx, NewThread{Provider: claude.id, Model: "opus", RuntimeMode: "approval-required"})
	if err != nil {
		t.Fatal(err)
	}

	child, err := m.Spawn(ctx, parent.ID, Subagent{Prompt: "Revisa el login\ncon detalle", Provider: "codex", Model: "gpt-6.2-sol"})
	if err != nil {
		t.Fatal(err)
	}
	turn := <-codex.turns
	if child.ParentID == nil || *child.ParentID != parent.ID || child.Title != "Revisa el login" || child.RuntimeMode != "approval-required" || turn.Model != "gpt-6.2-sol" {
		t.Fatalf("child %+v turn %+v", child, turn)
	}
	if parentDir, _ := m.Dir(ctx, parent.ID); turn.Dir != parentDir {
		t.Fatalf("child dir %q, parent %q", turn.Dir, parentDir)
	}
	if _, err = m.Spawn(ctx, child.ID, Subagent{Prompt: "nieto"}); !errors.Is(err, data.ErrInvalid) {
		t.Fatalf("grandchild accepted: %v", err)
	}

	events := <-codex.ends
	events <- Event{Message: &Message{ID: turn.RunID + ":a", Role: "assistant", Text: "Login correcto"}}
	events <- Event{Done: true}
	close(events)

	wake := <-claude.turns
	if !strings.Contains(wake.Text, "Revisa el login") || !strings.Contains(wake.Text, "terminó") || !strings.Contains(wake.Text, "Login correcto") {
		t.Fatalf("parent woke with %q", wake.Text)
	}
	close(<-claude.ends)
	wait(t, store, parent.ID, "idle")
	msgs, _ := m.Messages(ctx, parent.ID)
	if len(msgs) == 0 || msgs[0].Role != "user" || msgs[0].SenderThreadID != child.ID {
		t.Fatalf("wake message not attributed to the child: %+v", msgs)
	}
}

func TestSpawnLimitsActiveChildren(t *testing.T) {
	ctx := context.Background()
	m, store := manager(t, 0)
	parent, _ := m.Create(ctx, NewThread{Provider: "replay", Model: "grabacion"})
	for range MaxActiveChildren {
		child, err := m.Create(ctx, NewThread{Provider: "replay", ParentID: &parent.ID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = store.PatchThread(ctx, child.ID, map[string]any{"status": "working"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Spawn(ctx, parent.ID, Subagent{Prompt: "uno más"}); !errors.Is(err, data.ErrInvalid) {
		t.Fatalf("limit not enforced: %v", err)
	}
}
