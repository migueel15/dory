package runtime

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestUsageLedgerStoresCountersAndPreservesTotals(t *testing.T) {
	m, _ := manager(t, 0)
	ctx := context.Background()
	l := newLedger("hello user", 40, 0)
	tool := &ToolCall{Kind: "mcp", Server: "dory", Tool: "session_start"}
	l.message(Message{ID: "call", Tool: tool, InputChars: 20})
	l.message(Message{ID: "call", Tool: tool, Status: "completed", ResultChars: 200})
	l.step(StepUsage{Model: "gpt-6.2-sol", Input: 100, CacheRead: 10, Output: 10})
	part := l.parts[[2]string{catTool, "mcp:dory/session_start"}]
	if part == nil || part.Input != 0 {
		t.Fatalf("tool result charged to the call that issued it: %+v", part)
	}
	l.message(Message{ID: "answer", Role: "assistant", Text: "five words in this answer"})
	l.step(StepUsage{Model: "gpt-6.2-sol", Input: 160, CacheRead: 20, CacheWrite: 4, Output: 20, Reasoning: 5})
	if part.Input <= 0 || part.Output != 10 {
		t.Fatalf("tool result or arguments missing: %+v", part)
	}
	at := time.Now().UTC()
	for range 2 {
		if err := l.save(ctx, m.db, "thread", "run", "codex", at); err != nil {
			t.Fatal(err)
		}
	}
	var steps, reasoning int
	if err := m.db.QueryRow(`SELECT COUNT(*), SUM(reasoning) FROM usage_steps`).Scan(&steps, &reasoning); err != nil || steps != 2 || reasoning != 5 {
		t.Fatalf("steps=%d reasoning=%d err=%v", steps, reasoning, err)
	}
	turns, err := m.ThreadUsageTurns(ctx, "thread", at.Add(-time.Second))
	if err != nil || len(turns) != 1 {
		t.Fatalf("turns = %+v, err = %v", turns, err)
	}
	calls := turns[0].Calls
	if len(calls) != 2 || calls[0].Ordinal != 0 || calls[0].Input != 100 ||
		calls[1].Ordinal != 1 || calls[1].CacheWrite != 4 || calls[1].Reasoning != 5 {
		t.Fatalf("calls = %+v", calls)
	}

	rows, err := m.UsageBreakdown(ctx, at.Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	var got [4]float64
	for _, row := range rows {
		got[0] += row.Input
		got[1] += row.CacheRead
		got[2] += row.CacheWrite
		got[3] += row.Output
	}
	for i, want := range []float64{260, 30, 4, 30} {
		if math.Abs(got[i]-want) > 1e-9 {
			t.Errorf("token total %d = %g, want %g", i, got[i], want)
		}
	}
}

func TestClaudeLedgerUsesFinalOutputOnce(t *testing.T) {
	p := claudeParser{run: "run", texts: map[string]*Message{}, tools: map[string]*Message{}}
	var steps []StepUsage
	for _, line := range []string{
		`{"type":"stream_event","event":{"type":"message_start","message":{"id":"msg","model":"test","usage":{"input_tokens":20,"cache_read_input_tokens":80,"output_tokens":1}}}}`,
		`{"type":"assistant","message":{"id":"msg","model":"test","usage":{"input_tokens":20,"cache_read_input_tokens":80,"output_tokens":1}}}`,
		`{"type":"stream_event","event":{"type":"message_delta","usage":{"output_tokens":10}}}`,
		`{"type":"assistant","message":{"id":"msg","model":"test","usage":{"input_tokens":20,"cache_read_input_tokens":80,"output_tokens":1}}}`,
		`{"type":"result","subtype":"success"}`,
	} {
		p.parse([]byte(line), func(e Event) {
			if e.Step != nil {
				steps = append(steps, *e.Step)
			}
		})
	}
	want := StepUsage{Model: "test", Input: 20, CacheRead: 80, Output: 10}
	if len(steps) != 1 || steps[0] != want {
		t.Fatalf("steps = %+v, want %+v", steps, want)
	}
}
