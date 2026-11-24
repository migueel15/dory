package codex

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit/goldentest"
)

type buffer struct {
	bytes.Buffer
	closed bool
}

func (b *buffer) Close() error { b.closed = true; return nil }

// The recording is a real codex app-server 0.169 turn that asks with
// request_user_input; the test answers it the moment it arrives.
func TestSessionAsks(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "codex-app.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	in := &buffer{}
	s := newCodexSession(harness.Turn{RunID: "r1", Dir: "/tmp/x", Text: "hola"}, in)
	s.call("initialize", nil)
	var events []harness.Event
	var req *harness.Request
	for _, line := range bytes.Split(raw, []byte("\n")) {
		s.handle(line, func(e harness.Event) {
			events = append(events, e)
			if r, ok := e.(harness.RequestOpened); ok {
				req = &r.Request
				if err := s.Respond(req.ID, [][]string{{"Blue"}}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	goldentest.Check(t, "codex-app", events)
	var calls []string
	var answer string
	for _, l := range strings.Split(strings.TrimSpace(in.String()), "\n") {
		var r struct {
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
	if req == nil || req.ID != "call_e7a712e8c08e4669868fc50a715562d6" || req.Questions[0].ID != "preferred_color" {
		t.Fatalf("request: %+v", req)
	}
	// The process serves the next turn of the thread, so stdin stays open.
	if end, ok := events[len(events)-1].(harness.TurnEnded); !ok || end.Reason != harness.Completed || in.closed {
		t.Fatalf("end: %+v closed=%v", events[len(events)-1], in.closed)
	}
}

func TestHidesMemoryCitations(t *testing.T) {
	s := newCodexSession(harness.Turn{RunID: "run"}, nil)
	var visible string
	notify := func(method string, params map[string]any, want string, emitted bool) {
		t.Helper()
		raw, err := json.Marshal(params)
		if err != nil {
			t.Fatal(err)
		}
		got := false
		s.notification(codexRPC{Method: method, Params: raw}, func(e harness.Event) {
			got = true
			switch e := e.(type) {
			case harness.TextDelta:
				visible += e.Delta
			case harness.TextDone:
				visible = e.Text
			}
		})
		if got != emitted || visible != want {
			t.Fatalf("%s: event = %t, text = %q; want event = %t, text = %q", method, got, visible, emitted, want)
		}
	}
	item := func(text string) map[string]any {
		return map[string]any{"item": map[string]any{"id": "msg", "type": "agentMessage", "text": text}}
	}
	delta := func(text, want string, emitted bool) {
		t.Helper()
		notify("item/agentMessage/delta", map[string]any{"itemId": "msg", "delta": text}, want, emitted)
	}
	notify("item/started", item(""), "", false)
	delta("Respuesta\n<oai-mem", "Respuesta\n", true)
	delta("-citation>\n<citation_entries>dato</citation_entries>", "Respuesta\n", false)
	delta("</oai-mem-", "Respuesta\n", false)
	delta("citation>\nFin", "Respuesta\n\nFin", true)
	notify("item/completed", item("Respuesta\n<oai-mem-citation>dato</oai-mem-citation>\nFin"), "Respuesta\n\nFin", true)
}
