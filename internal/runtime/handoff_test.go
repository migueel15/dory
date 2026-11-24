package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/harness"
)

type capturedProvider struct {
	id    string
	turns chan Turn
	ends  chan chan Event
}

func (p capturedProvider) Info() harness.Info { return harness.Info{ID: p.id, Driver: p.id} }

func (p capturedProvider) Usage(context.Context) ([]Window, error) { return nil, ErrNoUsage }

func (p capturedProvider) Detect(context.Context) Status {
	return Status{ID: p.id, Installed: true}
}

func (p capturedProvider) Start(_ context.Context, turn Turn) (Session, error) {
	events := make(chan Event, 3)
	p.turns <- turn
	p.ends <- events
	return harness.Adapt(events, rejectResponse), nil
}

func TestSwitchProviderKeepsThreadAndHandsOffText(t *testing.T) {
	ctx := context.Background()
	m, store := manager(t, 0)
	claude := capturedProvider{"claude", make(chan Turn, 1), make(chan chan Event, 1)}
	codex := capturedProvider{"codex", make(chan Turn, 1), make(chan chan Event, 1)}
	m.registry.Register(claude)
	m.registry.Register(codex)

	thread, err := m.Create(ctx, NewThread{Provider: claude.id, Model: "haiku"})
	if err != nil {
		t.Fatal(err)
	}
	run := func(provider capturedProvider, text string) (Turn, chan Event) {
		t.Helper()
		if err := m.Send(ctx, thread.ID, text); err != nil {
			t.Fatal(err)
		}
		return <-provider.turns, <-provider.ends
	}
	finish := func(turn Turn, events chan Event, session, answer string) {
		t.Helper()
		events <- Event{Session: session}
		events <- Event{Message: &Message{ID: turn.RunID + ":assistant", Role: "assistant", Text: answer}}
		events <- Event{Done: true}
		close(events)
		wait(t, store, thread.ID, "idle")
	}
	first, events := run(claude, "Acuerda usar azul")
	target, model, effort := codex.id, "gpt-6.2-sol", "high"
	selection := ThreadSettings{Provider: &target, Model: &model, Effort: &effort}
	if _, err := m.Configure(ctx, thread.ID, selection); !errors.Is(err, ErrBusy) {
		t.Fatalf("switch during a turn: %v", err)
	}
	events <- Event{Settings: &ReportedSettings{Model: "claude-haiku-5-5"}}
	finish(first, events, "claude-session", "Usaremos azul")

	updated, err := m.Configure(ctx, thread.ID, selection)
	if err != nil || updated.ID != thread.ID || updated.Provider != target {
		t.Fatalf("switch: %+v, %v", updated, err)
	}
	if last := m.LastTurnModel(ctx, thread.ID); last == nil || last.Provider != claude.id || last.ReportedModel != "claude-haiku-5-5" {
		t.Fatalf("selecting a provider changed the last applied model: %+v", last)
	}
	updates, unsubscribe := m.Subscribe(thread.ID)
	defer unsubscribe()
	second, events := run(codex, "¿Qué color acordamos?")
	if second.Session != "" || second.Dir != first.Dir || second.Model != model || second.Effort != effort {
		t.Fatalf("destination turn: %+v", second)
	}
	if !strings.Contains(second.Text, `{"role":"user","text":"Acuerda usar azul"}`) ||
		!strings.Contains(second.Text, `{"role":"assistant","text":"Usaremos azul"}`) ||
		!strings.HasSuffix(second.Text, "Solicitud nueva:\n¿Qué color acordamos?") {
		t.Fatalf("handoff: %s", second.Text)
	}
	finish(second, events, "codex-session", "Azul")
	if last := m.LastTurnModel(ctx, thread.ID); last == nil || last.Provider != codex.id || last.Model == nil || *last.Model != model {
		t.Fatalf("last applied model after sending: %+v", last)
	}
	var streamed *TurnModel
	for len(updates) > 0 {
		if update := <-updates; update.LastTurnModel != nil {
			streamed = update.LastTurnModel
		}
	}
	if streamed == nil || streamed.Provider != codex.id || streamed.Model == nil || *streamed.Model != model {
		t.Fatalf("streamed applied model: %+v", streamed)
	}
	third, events := run(codex, "Continúa")
	if third.Session != "codex-session" || third.Text != "Continúa" {
		t.Fatalf("native resume: %+v", third)
	}
	finish(third, events, "codex-session", "Entendido")

	model = "gpt-6.2-astra"
	if _, err := m.Configure(ctx, thread.ID, selection); err != nil {
		t.Fatal(err)
	}
	sameProvider, events := run(codex, "Otro modelo, misma sesión")
	if sameProvider.Session != "codex-session" || sameProvider.Text != "Otro modelo, misma sesión" {
		t.Fatalf("same-provider turn: %+v", sameProvider)
	}
	finish(sameProvider, events, "codex-session", "Sigo en el hilo")

	target, model = claude.id, "haiku"
	if _, err := m.Configure(ctx, thread.ID, selection); err != nil {
		t.Fatal(err)
	}
	back, events := run(claude, "Explica la decisión")
	if back.Session != "" || !strings.Contains(back.Text, `"text":"Azul"`) {
		t.Fatalf("return handoff: %+v", back)
	}
	finish(back, events, "new-claude-session", "La decisión fue azul")
	messages, err := m.Messages(ctx, thread.ID)
	if err != nil || len(messages) != 13 || messages[3].Text != "¿Qué color acordamos?" {
		t.Fatalf("visible transcript: %+v, %v", messages, err)
	}
	for _, want := range []struct {
		index    int
		from, to string
		handoff  bool
	}{{2, "haiku", "gpt-6.2-sol", true}, {7, "gpt-6.2-sol", "gpt-6.2-astra", false}, {10, "gpt-6.2-astra", "haiku", true}} {
		msg := messages[want.index]
		var marker modelChange
		if msg.Role != "model-change" || json.Unmarshal([]byte(msg.Detail), &marker) != nil ||
			marker.From.Model == nil || *marker.From.Model != want.from ||
			marker.To.Model == nil || *marker.To.Model != want.to || marker.Handoff != want.handoff {
			t.Fatalf("transition at %d: %+v", want.index, msg)
		}
	}
	var firstChange modelChange
	if err := json.Unmarshal([]byte(messages[2].Detail), &firstChange); err != nil || firstChange.From.ReportedModel != "claude-haiku-5-5" {
		t.Fatalf("reported source model: %+v, %v", firstChange, err)
	}
	var portable bool
	if err := m.db.QueryRowContext(ctx, "SELECT portable_history FROM thread_runtime WHERE thread_id = ?", thread.ID).Scan(&portable); err != nil || !portable {
		t.Fatalf("portable history: %v, %v", portable, err)
	}
}

func TestPortablePromptKeepsOriginalAndRecentText(t *testing.T) {
	history := []Message{
		{Role: "system", Text: "internal prompt"},
		{Role: "user", Text: "original request"},
		{Role: "assistant", Text: strings.Repeat("x", handoffHistoryBytes)},
		{Role: "reasoning", Text: "internal reasoning"},
		{Role: "assistant", Text: "recent answer"},
	}
	prompt, ok := portablePrompt(history, "next request")
	if !ok || !strings.Contains(prompt, "original request") || !strings.Contains(prompt, "recent answer") ||
		!strings.Contains(prompt, "omitidos por espacio: 1") || !strings.HasSuffix(prompt, "next request") ||
		strings.Contains(prompt, "internal") || strings.Contains(prompt, strings.Repeat("x", 100)) {
		t.Fatalf("unexpected handoff: %q", prompt)
	}
}

func TestMergeModelChangesKeepsDividerBeforeNativeTurn(t *testing.T) {
	msgs := []Message{
		{ID: "old-question", Role: "user", CreatedAt: "2026-11-17T21:00:00Z"},
		{ID: "old-answer", Role: "assistant", CreatedAt: "2026-11-17T21:00:01Z"},
		{ID: "new-question", Role: "user", CreatedAt: "2026-11-17T21:00:02.123Z"},
	}
	markers := []Message{
		{ID: "divider", Role: "model-change", CreatedAt: "2026-11-17T21:00:02.123456Z"},
	}
	result := mergeModelChanges(msgs, markers)
	if len(result) != 4 || result[1].ID != "old-answer" || result[2].ID != "divider" || result[3].ID != "new-question" {
		t.Fatalf("native history order: %+v", result)
	}
}
