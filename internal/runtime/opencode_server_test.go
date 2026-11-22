package runtime

import (
	"reflect"
	"testing"
)

// The recording is a real opencode serve 2.3 session whose question tool
// opened a form, answered with "Blue".
func TestOpenCodeEventsAsk(t *testing.T) {
	p := newOpenCodeEvents("r1")
	events := parseFile(t, "opencode-serve.jsonl", func(line []byte, emit func(Event)) {
		p.handle("ses_e11afe441ffeZFhNs6jAXPCuXv", line, emit)
	})
	var req *Request
	var steps []StepUsage
	for _, e := range events {
		if e.Request != nil {
			req = e.Request
		}
		if e.Step != nil {
			steps = append(steps, *e.Step)
		}
	}
	wantSteps := []StepUsage{{Model: "openai/gpt-6.2-astra", Input: 10463, Output: 57}, {Model: "openai/gpt-6.2-astra", Input: 10557, Output: 5}}
	if !reflect.DeepEqual(steps, wantSteps) {
		t.Fatalf("usage steps = %+v", steps)
	}
	if req == nil || req.ID != "frm_1ee50396b001pmpd6IsSWgo56S" || req.MessageID != "r1:call_92b74c10d9bf45839a397a67bf26805f" {
		t.Fatalf("request: %+v", req)
	}
	q := req.Questions[0]
	if q.ID != "q0" || q.Question != "Which color do you prefer?" || q.Header != "Color preference" || len(q.Options) != 2 {
		t.Fatalf("question: %+v", q)
	}
	if f := p.fields[req.ID]; len(f) != 1 || f[0].Key != "q0" || f[0].Type != "string" {
		t.Fatalf("fields: %+v", f)
	}
	msgs, _ := final(events)
	tool := msgs[req.MessageID]
	if tool.Tool == nil || tool.Tool.Kind != "question" || tool.Status != "completed" || tool.Text != "" ||
		!reflect.DeepEqual(tool.Tool.Questions[0].Answer, []string{"Blue"}) {
		t.Fatalf("tool: %+v %+v", tool, tool.Tool)
	}
	if m := msgs["r1:msg_1ee5039a2001XzmdmmypN4RKJy:0"]; m.Role != "assistant" || m.Text != "Blue" || m.Streaming {
		t.Fatalf("answer: %+v", m)
	}
	if last := events[len(events)-1]; !last.Done || last.Err != "" {
		t.Fatalf("end: %+v", last)
	}
}

func TestOpenCodeEventsOtherSession(t *testing.T) {
	p := newOpenCodeEvents("r1")
	events := parseFile(t, "opencode-serve.jsonl", func(line []byte, emit func(Event)) {
		p.handle("ses_other", line, emit)
	})
	if len(events) != 0 {
		t.Fatalf("events of another session: %d", len(events))
	}
}
