package harness

import (
	"reflect"
	"testing"
)

func TestUpgrade(t *testing.T) {
	call := ToolCall{Kind: "command", Command: "pwd"}
	todos := []Todo{{Text: "Comprobar", Status: "pending"}}
	plan := ToolCall{Kind: "plan", Todos: todos}
	request := Request{ID: "question"}
	step := StepUsage{Model: "model", Input: 10}
	windows := []Window{{Kind: "weekly"}}
	meta := ItemMeta{RunID: "run", CreatedAt: "time"}
	update := ToolUpdated{ItemID: "tool", Call: &call, Status: "running", Result: "/repo", ResultChars: 5}
	doneUpdate := update
	doneUpdate.Status = "completed"
	cases := []struct {
		in  LegacyEvent
		out Event
	}{
		{LegacyEvent{Message: &Message{ID: "text", Role: "assistant", RunID: "run", CreatedAt: "time", Streaming: true, Text: "ho"}},
			TextDelta{ItemID: "text", Kind: Assistant, Delta: "ho", Meta: meta}},
		{LegacyEvent{Message: &Message{ID: "text", Role: "assistant", Streaming: true, Text: "hola"}},
			TextDelta{ItemID: "text", Kind: Assistant, Delta: "la"}},
		{LegacyEvent{Message: &Message{ID: "text", Role: "assistant", Streaming: true, Text: "adiós"}},
			TextDelta{ItemID: "text", Kind: Assistant, Delta: "adiós", Replace: true}},
		{LegacyEvent{Message: &Message{ID: "text", Role: "assistant", Text: "adiós"}},
			TextDone{ItemID: "text", Kind: Assistant, Text: "adiós"}},
		{LegacyEvent{Message: &Message{ID: "thought", Role: "reasoning", Text: "idea"}},
			TextDone{ItemID: "thought", Kind: Reasoning, Text: "idea"}},
		{LegacyEvent{Message: &Message{ID: "tool", Role: "tool", Status: "running", Tool: &call}},
			ToolStarted{ItemID: "tool", Call: call}},
		{LegacyEvent{Message: &Message{ID: "tool", Role: "tool", Status: "running", Tool: &call, Text: "/repo", ResultChars: 5}},
			update},
		{LegacyEvent{Message: &Message{ID: "tool", Role: "tool", Status: "completed", Tool: &call, Text: "/repo", ResultChars: 5}},
			ToolDone{ItemID: "tool", Status: "completed", Call: &call, Result: "/repo", ResultChars: 5, Update: &doneUpdate}},
		{LegacyEvent{Message: &Message{ID: "plan", Role: "plan", Title: "Plan", Tool: &plan}},
			PlanUpdated{ItemID: "plan", Title: "Plan", Todos: todos, Call: &plan}},
		{LegacyEvent{Message: &Message{Role: "user", Text: "Previous message"}}, MessageReported{Message{Role: "user", Text: "Previous message"}}},
		{LegacyEvent{Request: &request}, RequestOpened{request}},
		{LegacyEvent{Session: "session"}, SessionBound{"session"}},
		{LegacyEvent{Settings: &ReportedSettings{Model: "model", Effort: "high"}}, SettingsReported{Model: "model", Effort: "high"}},
		{LegacyEvent{Usage: windows}, UsageReported{windows}},
		{LegacyEvent{Step: &step}, step},
		{LegacyEvent{Steps: []StepUsage{step}}, StepsUsage{[]StepUsage{step}}},
		{LegacyEvent{Done: true}, TurnEnded{Reason: Completed}},
		{LegacyEvent{Done: true, Err: "interrumpido"}, TurnEnded{Interrupted, "interrumpido"}},
		{LegacyEvent{Done: true, Err: "el agente terminó sin respuesta"}, TurnEnded{NoResponse, "el agente terminó sin respuesta"}},
		{LegacyEvent{Done: true, Err: "exit 1"}, TurnEnded{Failed, "exit 1"}},
	}
	in := make(chan LegacyEvent, len(cases))
	for _, c := range cases {
		in <- c.in
	}
	close(in)
	out := Upgrade(in)
	for i, c := range cases {
		if got := <-out; !reflect.DeepEqual(got, c.out) {
			t.Errorf("event %d: got %#v, want %#v", i, got, c.out)
		}
	}
	if e, open := <-out; open {
		t.Fatalf("unexpected event %#v", e)
	}
}
