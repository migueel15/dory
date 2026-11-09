package t3

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"
)

// rpcServer answers one RPC per connection with the given exit.
func rpcServer(t *testing.T, exit string, seen *map[string]any) Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/websocket-ticket/rpc", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"ticket":"tk"}`))
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("wsTicket") != "tk" || r.URL.Query().Get("orchestrationProtocol") != "2" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		_, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		if seen != nil {
			json.Unmarshal(data, seen)
		}
		conn.Write(r.Context(), websocket.MessageText, []byte(`{"_tag":"Ping"}`))
		conn.Read(r.Context())
		conn.Write(r.Context(), websocket.MessageText, []byte(`[{"_tag":"Exit","requestId":"1","exit":`+exit+`}]`))
		conn.Read(r.Context())
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return Client{BaseURL: s.URL, Token: "good", HTTP: s.Client()}
}

func TestDispatchSendsCommandAndFillsID(t *testing.T) {
	var seen map[string]any
	c := rpcServer(t, `{"_tag":"Success","value":{"sequence":1}}`, &seen)
	if err := c.RespondApproval(context.Background(), "th", "rq", DecisionDecline); err != nil {
		t.Fatal(err)
	}
	p := seen["payload"].(map[string]any)
	if seen["tag"] != "orchestration.dispatchCommand" || p["type"] != "runtime-request.respond" || p["decision"] != "decline" || p["commandId"] == "" {
		t.Errorf("sent %+v", seen)
	}
}

func TestDispatchFailureAndBadToken(t *testing.T) {
	c := rpcServer(t, `{"_tag":"Failure","cause":[{"_tag":"Die","defect":"boom"}]}`, nil)
	if err := c.SendMessage(context.Background(), "th", "hola"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v", err)
	}
	c.Token = "bad"
	if err := c.SendMessage(context.Background(), "th", "hola"); err != errUnauthorized {
		t.Errorf("token: %v", err)
	}
}

func TestContextFindsExactCommandAndQuestions(t *testing.T) {
	c := rpcServer(t, `{"_tag":"Success","value":{
		"messages":[{"role":"user","text":"hola"}],
		"runtimeRequests":[
			{"id":"old","kind":"command","status":"resolved","nativeRequestRef":{"nativeId":"x"}},
			{"id":"rq","kind":"command","status":"pending","nativeRequestRef":{"nativeId":"n"}}],
		"turnItems":[
			{"type":"command_execution","nativeItemRef":{"nativeId":"n"},"input":"touch a"},
			{"type":"approval_request","nativeItemRef":{"nativeId":"n"},"requestId":"rq"}]}}`, nil)
	got, err := c.ThreadContext(context.Background(), "th")
	if err != nil {
		t.Fatal(err)
	}
	if got.Pending == nil || got.Pending.ID != "rq" || got.Pending.Detail != "touch a" || len(got.Messages) != 1 {
		t.Errorf("got %+v", got)
	}
	q := wireProjection{}
	json.Unmarshal([]byte(`{"runtimeRequests":[{"id":"q","kind":"user_input","status":"pending"}],
		"turnItems":[{"type":"user_input_request","requestId":"q","questions":[{"id":"c","question":"¿color?","multiSelect":true,"options":[{"label":"Rojo"}]}]}]}`), &q)
	p := q.context(contextMessages).Pending
	if len(p.Questions) != 1 || !p.Questions[0].Multi || p.Questions[0].Options[0].Label != "Rojo" {
		t.Errorf("questions %+v", p)
	}
}

func TestArchiveThreadDispatch(t *testing.T) {
	for _, fails := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fails], func(t *testing.T) {
			exit := `{"_tag":"Success","value":{"sequence":1}}`
			if fails {
				exit = `{"_tag":"Failure","cause":[{"_tag":"Die","defect":"offline"}]}`
			}
			var seen map[string]any
			c := rpcServer(t, exit, &seen)
			err := c.ArchiveThread(context.Background(), "thread")
			if (err != nil) != fails {
				t.Fatalf("ArchiveThread error=%v, failure expected=%v", err, fails)
			}
			p := seen["payload"].(map[string]any)
			if seen["tag"] != "orchestration.dispatchCommand" || p["type"] != "thread.archive" || p["threadId"] != "thread" || p["commandId"] == "" {
				t.Fatalf("unexpected archive command: %+v", seen)
			}
		})
	}
}

func TestCreateWorktree(t *testing.T) {
	var seen map[string]any
	c := rpcServer(t, `{"_tag":"Success","value":{"worktree":{"path":"/w/x","refName":"origin/main"}}}`, &seen)
	path, err := c.CreateWorktree(context.Background(), "/repo", "origin/main")
	if err != nil || path != "/w/x" {
		t.Fatalf("path=%q err=%v", path, err)
	}
	p := seen["payload"].(map[string]any)
	if seen["tag"] != "vcs.createWorktree" || p["cwd"] != "/repo" || p["refName"] != "origin/main" {
		t.Fatalf("unexpected request: %+v", seen)
	}
	if _, ok := p["newRefName"]; ok {
		t.Fatalf("must not create a branch: %+v", p)
	}
	if v, ok := p["path"]; !ok || v != nil {
		t.Fatalf("path must be null: %+v", p)
	}
	c = rpcServer(t, `{"_tag":"Success","value":{"worktree":{}}}`, nil)
	if _, err := c.CreateWorktree(context.Background(), "/repo", "origin/main"); err == nil {
		t.Fatal("empty path must fail")
	}
}
