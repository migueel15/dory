package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/harness"
)

// fakeAgent is an ACP server in a goroutine, spoken to through io.Pipe.
type fakeAgent struct {
	t    *testing.T
	out  *json.Encoder
	mu   sync.Mutex
	got  chan rpcFrame
	conn *conn
}

func newFakeAgent(t *testing.T, ctx context.Context, serve func(a *fakeAgent, f rpcFrame)) *fakeAgent {
	t.Helper()
	toClient, agentOut := io.Pipe()
	agentIn, toAgent := io.Pipe()
	ctx, cancel := context.WithCancel(ctx)
	a := &fakeAgent{t: t, out: json.NewEncoder(agentOut), got: make(chan rpcFrame, 64)}
	a.conn = newConn(ctx, cancel, "Fake", toAgent, toClient, func() error { return nil })
	go func() {
		defer agentOut.Close()
		scan := bufio.NewScanner(agentIn)
		scan.Buffer(make([]byte, 64<<10), 4<<20)
		for scan.Scan() {
			var f rpcFrame
			if json.Unmarshal(scan.Bytes(), &f) != nil {
				continue
			}
			a.got <- f
			serve(a, f)
		}
	}()
	t.Cleanup(func() { cancel(); toAgent.Close(); agentIn.Close() })
	return a
}

func (a *fakeAgent) send(v params) {
	a.mu.Lock()
	defer a.mu.Unlock()
	v["jsonrpc"] = "2.0"
	_ = a.out.Encode(v)
}

func (a *fakeAgent) result(f rpcFrame, result any) { a.send(params{"id": f.ID, "result": result}) }

func (a *fakeAgent) update(session string, u params) {
	a.send(params{"method": "session/update", "params": params{"sessionId": session, "update": u}})
}

func collect(s *session) (func() []harness.Message, func() []event) {
	var mu sync.Mutex
	var msgs []harness.Message
	var all []event
	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range s.events {
			mu.Lock()
			all = append(all, e)
			if e.Message != nil {
				msgs = append(msgs, *e.Message)
			}
			mu.Unlock()
		}
	}()
	wait := func() { <-done }
	return func() []harness.Message { wait(); return msgs }, func() []event { wait(); return all }
}

func fakeServe(permission *string, cancelled *bool) func(*fakeAgent, rpcFrame) {
	return func(a *fakeAgent, f rpcFrame) {
		switch f.Method {
		case "initialize":
			a.result(f, params{"protocolVersion": 1, "agentInfo": params{"name": "fake"}, "agentCapabilities": params{"loadSession": true}})
		case "session/new":
			a.result(f, params{"sessionId": "s1", "configOptions": []any{}})
		case "session/prompt":
			go func() {
				a.update("s1", params{"sessionUpdate": "agent_message_chunk", "content": params{"type": "text", "text": "Hel"}})
				a.update("s1", params{"sessionUpdate": "agent_message_chunk", "content": params{"type": "text", "text": "lo"}})
				a.update("s1", params{"sessionUpdate": "tool_call", "toolCallId": "t1", "kind": "execute", "title": "ls", "status": "in_progress", "rawInput": params{"command": "ls"}})
				a.update("s1", params{"sessionUpdate": "tool_call_update", "toolCallId": "t1", "status": "completed", "rawOutput": "a.txt"})
				a.send(params{"id": 99, "method": "session/request_permission", "params": params{"sessionId": "s1",
					"toolCall": params{"toolCallId": "t2", "kind": "execute", "title": "rm"},
					"options":  []params{{"kind": "reject_once", "optionId": "no", "name": "No"}, {"kind": "allow_once", "optionId": "yes", "name": "Yes"}}}})
				if cancelled != nil {
					// Wait for the client to cancel before ending the prompt.
					for g := range a.got {
						if g.Method == "session/cancel" {
							*cancelled = true
							break
						}
					}
					a.result(f, params{"stopReason": "cancelled"})
					return
				}
				for g := range a.got {
					if string(g.ID) == "99" {
						*permission = string(g.Result)
						break
					}
				}
				a.result(f, params{"stopReason": "end_turn"})
			}()
		}
	}
}

func TestFakeServerTurn(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for _, full := range []bool{true, false} {
		var permission string
		a := newFakeAgent(t, ctx, fakeServe(&permission, nil))
		d := &driver{p: Profile{ID: "fake", Name: "Fake"}}
		s := newSession(ctx, a.conn, harness.Turn{RunID: "r", Dir: t.TempDir(), Text: "hi", FullAccess: full}, "fake-default")
		messages, _ := collect(s)
		err := d.run(s)
		s.finishChunk()
		close(s.events)
		if err != nil {
			t.Fatal(err)
		}
		var text string
		var tool harness.Message
		for _, m := range messages() {
			switch {
			case m.Role == "assistant" && !m.Streaming:
				text = m.Text
			case m.Role == "tool":
				tool = m
			}
		}
		if text != "Hello" {
			t.Errorf("assistant text %q", text)
		}
		if tool.Tool == nil || tool.Tool.Kind != "command" || tool.Tool.Command != "ls" || tool.Status != "completed" || tool.Detail != "a.txt" {
			t.Errorf("tool message %+v %+v", tool, tool.Tool)
		}
		want := `"optionId":"no"`
		if full {
			want = `"optionId":"yes"`
		}
		if !strings.Contains(permission, want) {
			t.Errorf("full=%v: permission answer %s, want %s", full, permission, want)
		}
	}
}

func TestFakeServerCancel(t *testing.T) {
	ctx, stop := context.WithTimeout(t.Context(), 10*time.Second)
	defer stop()
	turn, cancel := context.WithCancel(ctx)
	var cancelled bool
	a := newFakeAgent(t, ctx, fakeServe(nil, &cancelled))
	d := &driver{p: Profile{ID: "fake", Name: "Fake"}}
	s := newSession(turn, a.conn, harness.Turn{RunID: "r", Dir: t.TempDir(), Text: "hi", FullAccess: true}, "fake-default")
	go func() {
		// Cancel once the agent is working on the prompt.
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	_, events := collect(s)
	err := d.run(s)
	close(s.events)
	_ = events()
	if err == nil {
		t.Fatal("a cancelled prompt must report the cancellation")
	}
	if !cancelled {
		t.Fatal("the agent did not receive session/cancel")
	}
}

type fixtureInput struct{ bytes.Buffer }

func (*fixtureInput) Close() error { return nil }

func feed(t *testing.T, c *conn, name string) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		var frame rpcFrame
		if err := json.Unmarshal(scan.Bytes(), &frame); err != nil {
			t.Fatal(err)
		}
		c.frames <- frame
	}
	if err := scan.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestResumeTranscript(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	d := &driver{p: Profile{ID: "antigravity", Name: "Antigravity", Agents: []string{"Antigravity ACP Server"}, RequireLoad: true}}
	for _, full := range []bool{false, true} {
		input := &fixtureInput{}
		c := &conn{name: "Antigravity", in: input, frames: make(chan rpcFrame, 32)}
		feed(t, c, "initialize")
		for _, line := range []string{
			`{"id":2,"method":"session/update","params":{"sessionId":"saved","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"old message"}}}}`,
			`{"id":2,"result":{"configOptions":[{"id":"model","currentValue":"before"},{"id":"think","category":"thought_level","currentValue":"low"},{"id":"mode","currentValue":"plan"}]}}`,
			`{"id":3,"result":{}}`,
			`{"id":4,"result":{}}`,
			`{"id":5,"result":{}}`,
		} {
			var frame rpcFrame
			if err := json.Unmarshal([]byte(line), &frame); err != nil {
				t.Fatal(err)
			}
			c.frames <- frame
		}
		feed(t, c, "turn")
		s := newSession(ctx, c, harness.Turn{RunID: "turn", Dir: t.TempDir(), Session: "saved", Model: "after", Effort: "high", Text: "Hi", FullAccess: full, MCPURL: "http://127.0.0.1:8080/mcp?thread=one", MCPToken: "test"}, "antigravity-default")
		done := make(chan error, 1)
		go func() {
			done <- d.run(s)
			close(s.events)
		}()
		var last harness.Message
		var sawSession, sawEdit, sawPlan bool
		for event := range s.events {
			switch {
			case event.Session == "saved":
				sawSession = true
			case event.Request != nil:
				if err := s.Answer(event.Request.ID, [][]string{{"dev"}}); err != nil {
					t.Fatal(err)
				}
			case event.Message != nil:
				last = *event.Message
				if last.Text == "old message" {
					t.Fatal("resumed history was emitted as a live turn")
				}
				sawEdit = sawEdit || (last.Tool != nil && last.Tool.Kind == "edit" && last.Status == "completed")
				sawPlan = sawPlan || (last.Role == "plan" && strings.Contains(last.Text, "- [x] Inspect"))
			}
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if !sawSession || !sawEdit || !sawPlan || last.Text != "Hello, Dory" || last.Streaming {
			t.Fatalf("unexpected events: session %v, edit %v, plan %v, last %+v", sawSession, sawEdit, sawPlan, last)
		}
		var outbound []rpcFrame
		scan := bufio.NewScanner(input)
		for scan.Scan() {
			var frame rpcFrame
			if err := json.Unmarshal(scan.Bytes(), &frame); err != nil {
				t.Fatal(err)
			}
			outbound = append(outbound, frame)
		}
		wantExecute := "no"
		if full {
			wantExecute = "yes"
		}
		if len(outbound) != 9 || outbound[1].Method != "session/load" || !bytes.Contains(outbound[1].Params, []byte("mcp-stdio")) ||
			!bytes.Contains(outbound[6].Result, []byte(`"optionId":"yes"`)) ||
			!bytes.Contains(outbound[7].Result, []byte(`"optionId":"`+wantExecute+`"`)) ||
			!bytes.Contains(outbound[8].Result, []byte(`"optionId":"b"`)) {
			t.Fatalf("unexpected RPC frames: %+v", outbound)
		}
	}
}

func TestDeclaredProfile(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip(err)
	}
	d := New(Declared("mine", "", sh, []string{"--acp"}, nil), nil)
	if info := d.Info(); info.ID != "mine" || info.Name != "mine" || !info.Capabilities.Resume {
		t.Fatalf("info %+v", info)
	}
	if st := d.Detect(t.Context()); !st.Installed || len(st.Models) != 1 || st.Models[0].Slug != "mine-default" {
		t.Fatalf("status %+v", st)
	}
	if _, ok := d.(harness.Admin); ok {
		t.Fatal("a declared profile has no administration commands")
	}
	missing := New(Declared("gone", "Gone", "dory-no-such-agent", nil, nil), nil)
	if missing.Detect(t.Context()).Installed {
		t.Fatal("a missing command must not be installed")
	}
}

func TestWorktreePermissions(t *testing.T) {
	dir := t.TempDir()
	out := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, kind string
		paths      []string
		allowed    bool
	}{
		{"nested new file", "edit", []string{filepath.Join(dir, "src", "deep", "new.go")}, true},
		{"relative", "edit", []string{filepath.Join("src", "one.go")}, true},
		{"outside", "edit", []string{filepath.Join(out, "file")}, false},
		{"mixed", "edit", []string{"ok", filepath.Join(out, "file")}, false},
		{"missing target", "edit", nil, false},
		{"parent traversal", "edit", []string{filepath.Join("..", "outside")}, false},
		{"execution", "execute", []string{"ok"}, false},
		{"deletion", "delete", []string{"ok"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tool := toolCall{Kind: tt.kind}
			for _, path := range tt.paths {
				tool.Locations = append(tool.Locations, location{Path: path})
			}
			if got := worktreeEdit(dir, tool); got != tt.allowed {
				t.Fatalf("allowed=%v", got)
			}
		})
	}
	t.Run("symlink escape", func(t *testing.T) {
		link := filepath.Join(dir, "link")
		if err := os.Symlink(out, link); err != nil {
			t.Skip(err)
		}
		if worktreeEdit(dir, toolCall{Kind: "edit", Locations: []location{{Path: filepath.Join(link, "new")}}}) {
			t.Fatal("approved an edit outside the worktree through a symlink")
		}
	})
}
