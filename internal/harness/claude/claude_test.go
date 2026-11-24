package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit/goldentest"
)

type buffer struct {
	bytes.Buffer
	closed bool
}

func (b *buffer) Close() error { b.closed = true; return nil }

func TestGolden(t *testing.T) {
	for _, name := range []string{"claude", "claude-split"} {
		t.Run(name, func(t *testing.T) {
			parse := LineParser("r1")
			goldentest.Check(t, name, goldentest.Lines(t, name+".jsonl", parse))
		})
	}
}

// The recording is a real run of Claude Code 2.1 answering AskUserQuestion
// over --permission-prompt-tool stdio.
func TestSessionAsks(t *testing.T) {
	in := &buffer{}
	s := newClaudeSession("r1", in, true)
	events := goldentest.Lines(t, "claude-question.jsonl", s.handle)
	goldentest.Check(t, "claude-question", events)
	var req *harness.Request
	for _, e := range events {
		if r, ok := e.(harness.RequestOpened); ok {
			req = &r.Request
		}
	}
	if req == nil || req.Kind != "question" || req.MessageID != "toolu_012fnMxnrm9guShHDg3VdSMb" || len(req.Questions) != 1 {
		t.Fatalf("request: %+v", req)
	}
	if in.closed {
		t.Fatal("stdin closed after the result")
	}
	if err := s.Respond(req.ID, [][]string{{"Blue"}}); err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Response struct {
			RequestID string `json:"request_id"`
			Response  struct {
				Behavior     string `json:"behavior"`
				UpdatedInput struct {
					Answers map[string]string `json:"answers"`
				} `json:"updatedInput"`
			} `json:"response"`
		} `json:"response"`
	}
	if err := json.Unmarshal(in.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	r := resp.Response
	if r.RequestID != req.ID || r.Response.Behavior != "allow" || r.Response.UpdatedInput.Answers["Which color do you prefer?"] != "Blue" {
		t.Fatalf("response: %s", in.String())
	}
	if err := s.Respond(req.ID, nil); !errors.Is(err, harness.ErrNoRequest) {
		t.Fatalf("second answer: %v", err)
	}
}

func TestSessionDeniesWithoutAccess(t *testing.T) {
	in := &buffer{}
	s := newClaudeSession("r1", in, false)
	s.handle([]byte(`{"type":"control_request","request_id":"q1","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"ls"}}}`), func(harness.Event) {})
	if !bytes.Contains(in.Bytes(), []byte(`"behavior":"deny"`)) {
		t.Fatalf("response: %s", in.String())
	}
}

func TestLedgerUsesFinalOutputOnce(t *testing.T) {
	parse := LineParser("run")
	var steps []harness.StepUsage
	for _, line := range []string{
		`{"type":"stream_event","event":{"type":"message_start","message":{"id":"msg","model":"test","usage":{"input_tokens":20,"cache_read_input_tokens":80,"output_tokens":1}}}}`,
		`{"type":"assistant","message":{"id":"msg","model":"test","usage":{"input_tokens":20,"cache_read_input_tokens":80,"output_tokens":1}}}`,
		`{"type":"stream_event","event":{"type":"message_delta","usage":{"output_tokens":10}}}`,
		`{"type":"assistant","message":{"id":"msg","model":"test","usage":{"input_tokens":20,"cache_read_input_tokens":80,"output_tokens":1}}}`,
		`{"type":"result","subtype":"success"}`,
	} {
		parse([]byte(line), func(e harness.Event) {
			if s, ok := e.(harness.StepUsage); ok {
				steps = append(steps, s)
			}
		})
	}
	want := []harness.StepUsage{{Model: "test", Input: 20, CacheRead: 80, Output: 10}}
	if !reflect.DeepEqual(steps, want) {
		t.Fatalf("steps = %+v, want %+v", steps, want)
	}
}
