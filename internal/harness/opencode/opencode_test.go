package opencode

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit/goldentest"
)

// The recording is a real opencode serve 2.3 session whose question tool
// opened a form, answered with "Blue".
func TestEventsAsk(t *testing.T) {
	p := newOpenCodeEvents("r1")
	events := goldentest.Lines(t, "opencode-serve.jsonl", func(line []byte, emit func(harness.Event)) {
		p.handle("ses_e11afe441ffeZFhNs6jAXPCuXv", line, emit)
	})
	goldentest.Check(t, "opencode-serve", events)
	var req *harness.Request
	var steps []harness.StepUsage
	for _, e := range events {
		switch e := e.(type) {
		case harness.RequestOpened:
			req = &e.Request
		case harness.StepUsage:
			steps = append(steps, e)
		}
	}
	want := []harness.StepUsage{{Model: "openai/gpt-6.2-astra", Input: 10463, Output: 57}, {Model: "openai/gpt-6.2-astra", Input: 10557, Output: 5}}
	if !reflect.DeepEqual(steps, want) {
		t.Fatalf("usage steps = %+v", steps)
	}
	if req == nil || req.ID != "frm_1ee50396b001pmpd6IsSWgo56S" || req.MessageID != "r1:call_92b74c10d9bf45839a397a67bf26805f" {
		t.Fatalf("request: %+v", req)
	}
	if f := p.fields[req.ID]; len(f) != 1 || f[0].Key != "q0" || f[0].Type != "string" {
		t.Fatalf("fields: %+v", f)
	}
}

func TestEventsOtherSession(t *testing.T) {
	p := newOpenCodeEvents("r1")
	events := goldentest.Lines(t, "opencode-serve.jsonl", func(line []byte, emit func(harness.Event)) {
		p.handle("ses_other", line, emit)
	})
	if len(events) != 0 {
		t.Fatalf("events of another session: %d", len(events))
	}
}

func TestRegisterDory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "PUT /api/experimental/mcp/dory":
			var body struct {
				Config struct {
					URL     string
					Headers map[string]string
				}
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
				body.Config.URL != "http://localhost/mcp?thread=one" ||
				body.Config.Headers["Authorization"] != "Bearer thread-token" {
				t.Error("incorrect OpenCode MCP configuration")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case "GET /api/mcp":
			io.WriteString(w, `{"data":[{"name":"dory","status":{"status":"connected"}}]}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	api := openCodeAPI{base: server.URL}
	if err := api.registerDory(t.Context(), harness.Turn{MCPURL: "http://localhost/mcp?thread=one", MCPToken: "thread-token"}); err != nil {
		t.Fatal(err)
	}
}
