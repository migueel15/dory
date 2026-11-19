package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

type buffer struct {
	bytes.Buffer
	closed bool
}

func (b *buffer) Close() error { b.closed = true; return nil }

// The recording is a real run of Claude Code 2.1 answering AskUserQuestion
// over --permission-prompt-tool stdio.
func TestClaudeSessionAsks(t *testing.T) {
	in := &buffer{}
	s := newClaudeSession("r1", in, true)
	events := parseFile(t, "claude-question.jsonl", s.handle)
	var req *Request
	for _, e := range events {
		if e.Request != nil {
			req = e.Request
		}
	}
	if req == nil || req.Kind != "question" || req.MessageID != "toolu_012fnMxnrm9guShHDg3VdSMb" ||
		len(req.Questions) != 1 || req.Questions[0].Question != "Which color do you prefer?" || len(req.Questions[0].Options) != 2 {
		t.Fatalf("request: %+v", req)
	}
	if !in.closed {
		t.Fatal("stdin stays open after the result")
	}
	if err := s.Answer(req.ID, [][]string{{"Blue"}}); err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Type     string `json:"type"`
		Response struct {
			Subtype   string `json:"subtype"`
			RequestID string `json:"request_id"`
			Response  struct {
				Behavior     string `json:"behavior"`
				UpdatedInput struct {
					Questions []any             `json:"questions"`
					Answers   map[string]string `json:"answers"`
				} `json:"updatedInput"`
			} `json:"response"`
		} `json:"response"`
	}
	if err := json.Unmarshal(in.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	r := resp.Response
	if resp.Type != "control_response" || r.Subtype != "success" || r.RequestID != req.ID || r.Response.Behavior != "allow" ||
		len(r.Response.UpdatedInput.Questions) != 1 || r.Response.UpdatedInput.Answers["Which color do you prefer?"] != "Blue" {
		t.Fatalf("response: %s", in.String())
	}
	if err := s.Answer(req.ID, nil); !errors.Is(err, ErrNoRequest) {
		t.Fatalf("second answer: %v", err)
	}
	msgs, _ := final(events)
	q := msgs[req.MessageID]
	if q.Tool == nil || q.Tool.Kind != "question" || q.Status != "completed" || q.Text != "" ||
		!reflect.DeepEqual(q.Tool.Questions[0].Answer, []string{"Blue"}) {
		t.Fatalf("question message: %+v %+v", q, q.Tool)
	}
	if msgs["msg_011ChAGCCnKgiAzPKfpQBMUp:0"].Text != "Blue" {
		t.Fatal("the agent did not go on in the same turn")
	}
}

func TestClaudeSessionDeniesWithoutAccess(t *testing.T) {
	in := &buffer{}
	s := newClaudeSession("r1", in, false)
	s.handle([]byte(`{"type":"control_request","request_id":"q1","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"ls"}}}`), func(Event) {})
	if !bytes.Contains(in.Bytes(), []byte(`"behavior":"deny"`)) {
		t.Fatalf("response: %s", in.String())
	}
}

// asker is an interactive provider whose turn asks one question and waits.
type asker struct{}

func (asker) ID() string                              { return "asker" }
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
	return s, nil
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
	m.providers["asker"] = asker{}
	th, _ := m.Create(ctx, NewThread{Provider: "asker"})
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
