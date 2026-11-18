package runtime

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The recording is a real codex app-server 0.169 turn that asks with
// request_user_input; the test answers it the moment it arrives.
func TestCodexSessionAsks(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "codex-app.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	in := &buffer{}
	s := newCodexSession(Turn{RunID: "r1", Dir: "/tmp/x", Text: "hola"}, in)
	s.call("initialize", nil)
	var events []Event
	var req *Request
	for _, line := range bytes.Split(raw, []byte("\n")) {
		s.handle(line, func(e Event) {
			events = append(events, e)
			if e.Request != nil {
				req = e.Request
				if err := s.Answer(req.ID, [][]string{{"Blue"}}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	var calls []string
	var answer string
	for _, l := range strings.Split(strings.TrimSpace(in.String()), "\n") {
		var r struct {
			ID     any             `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
		}
		json.Unmarshal([]byte(l), &r)
		if r.Method != "" {
			calls = append(calls, r.Method)
		} else {
			answer = string(r.Result)
		}
	}
	if want := []string{"initialize", "initialized", "thread/start", "turn/start"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls: %v", calls)
	}
	if answer != `{"answers":{"preferred_color":{"answers":["Blue"]}}}` {
		t.Fatalf("answer: %s", answer)
	}
	if req == nil || req.ID != "call_e7a712e8c08e4669868fc50a715562d6" || req.Questions[0].ID != "preferred_color" || len(req.Questions[0].Options) != 2 {
		t.Fatalf("request: %+v", req)
	}
	if events[0].Session != "01a1ee4d-0862-7162-8921-3f4994d95588" {
		t.Fatalf("session: %+v", events[0])
	}
	msgs, _ := final(events)
	q := msgs[req.MessageID]
	if q.Status != "completed" || !reflect.DeepEqual(q.Tool.Questions[0].Answer, []string{"Blue"}) {
		t.Fatalf("question: %+v %+v", q, q.Tool)
	}
	if m := msgs["r1:msg_000a970bc8682c1a016afd84614c4087d2bd6e535796a43126"]; m.Text != "Blue" || m.Streaming {
		t.Fatalf("answer message: %+v", m)
	}
	if last := events[len(events)-1]; !last.Done || last.Err != "" || !in.closed {
		t.Fatalf("end: %+v closed=%v", last, in.closed)
	}
}
