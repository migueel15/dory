package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"
	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/internal/t3"
)

func TestReviewPrompt(t *testing.T) {
	got := reviewPrompt(7, "a/b.go", 3, 5, "+x\n-y\n", "¿Por qué?")
	want := "Consulta de revisión sobre PR #7, fichero a/b.go líneas 3–5:\n```diff\n+x\n-y\n```\n¿Por qué?\n\nResponde sin modificar código salvo que se pida."
	if got != want {
		t.Fatalf("got %q", got)
	}
	if got := reviewPrompt(7, "a", 4, 4, "", "q"); !strings.Contains(got, "fichero a líneas 4:") {
		t.Fatalf("single line: %q", got)
	}
	if got := reviewPrompt(7, "a.go", 0, 0, "", "q"); !strings.Contains(got, "fichero a.go completo:\nq") {
		t.Fatalf("whole file: %q", got)
	}
}

func TestReviewIntroEndsWithQuestion(t *testing.T) {
	pr := &github.Pull{Number: 7, Repo: "o/r", Title: "T", URL: "https://github.com/o/r/pull/7", HeadRef: "feat/x", BaseRef: "main"}
	got := reviewIntro(pr, "PREGUNTA")
	if !strings.HasSuffix(got, "\n\nPREGUNTA") || !strings.Contains(got, "Rama: feat/x → main") || strings.Contains(got, "%!") {
		t.Fatalf("intro: %q", got)
	}
}

// Embedding the service keeps these handler tests independent of PostgreSQL;
// any unexpected service call fails instead of silently succeeding.
type reviewTestService struct {
	service.Service
	links *reviewTestLinks
}

func (s reviewTestService) IssueThreads() service.Resource[service.IssueThread] { return s.links }

type reviewTestLinks struct {
	service.Resource[service.IssueThread]
	values []service.IssueThread
}

func (s *reviewTestLinks) List(_ context.Context, f service.Filter) ([]service.IssueThread, error) {
	if f.Fields["kind"] != "review" || (f.Fields["repo"] != nil && (f.Fields["repo"] != "o/r" || f.Fields["number"] != 7)) {
		panic("unexpected filter")
	}
	return s.values, nil
}
func TestCreateReviewThreadGuards(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		exists     bool
		status     int
	}{
		{"missing launch", `{}`, false, 400},
		{"incomplete launch", `{"launch":{"instance_id":"i"}}`, false, 400},
		{"existing thread", `{}`, true, 409},
		{"invalid json", `{`, false, 400},
	} {
		t.Run(tt.name, func(t *testing.T) {
			links := &reviewTestLinks{}
			if tt.exists {
				links.values = []service.IssueThread{{ThreadID: "existing"}}
			}
			mux := http.NewServeMux()
			mountReviewAsk(mux, reviewTestService{links: links}, nil, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/github/repos/o/r/pulls/7/review-thread", strings.NewReader(tt.body)))
			if w.Code != tt.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func (s reviewTestService) RecordEvent(context.Context, string, string, string, any) error {
	return nil
}
func (s *reviewTestLinks) Put(_ context.Context, _ string, v service.IssueThread) (service.IssueThread, error) {
	s.values = append(s.values, v)
	return v, nil
}
func (s *reviewTestLinks) Archive(context.Context, string, string) error {
	panic("review link must stay active")
}

type reviewTestGithub struct{ state string }

func (g reviewTestGithub) Detail(context.Context, string, string, int) (*github.Detail, error) {
	return &github.Detail{Pull: &github.Pull{ProjectID: "dory", Repo: "o/r", Number: 7, Title: "Example", URL: "https://github.com/o/r/pull/7", State: g.state, HeadRef: "feature", BaseRef: "main"}}, nil
}

type reviewTestMonitor struct{ client t3.Client }

func (m reviewTestMonitor) Thread(string) (t3.Thread, bool) { return t3.Thread{}, false }
func (m reviewTestMonitor) ThreadClient(context.Context, string) (t3.Client, error) {
	return t3.Client{}, t3.ErrThreadNotFound
}
func (m reviewTestMonitor) InstanceClient(context.Context, string) (t3.Client, error) {
	return m.client, nil
}
func (m reviewTestMonitor) Refresh(string) {}

// Exercise the real T3 client against its HTTP/WebSocket boundary.
func reviewRPC(t *testing.T, respond func(string, map[string]any) any) t3.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/websocket-ticket/", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"ticket":"test"}`)) })
	mux.HandleFunc("/api/auth/websocket-ticket", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"ticket":"test"}`)) })
	mux.HandleFunc("/api/orchestration/shell", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"projects":[{"id":"project","workspaceRoot":"/repo"}]}`))
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		var req struct {
			Tag     string         `json:"tag"`
			Payload map[string]any `json:"payload"`
		}
		_, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		if err := json.Unmarshal(data, &req); err != nil {
			t.Error(err)
			return
		}
		value := respond(req.Tag, req.Payload)
		out, _ := json.Marshal(map[string]any{"_tag": "Exit", "requestId": "1", "exit": map[string]any{"_tag": "Success", "value": value}})
		conn.Write(r.Context(), websocket.MessageText, out)
		conn.Read(r.Context())
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return t3.Client{BaseURL: server.URL, Token: "test", HTTP: server.Client()}
}

func TestCreateReviewThread(t *testing.T) {
	var threadID string
	var messages []string
	client := reviewRPC(t, func(tag string, p map[string]any) any {
		switch tag {
		case "server.getConfig":
			return map[string]any{}
		case "git.preparePullRequestThread":
			if p["reference"] != "https://github.com/o/r/pull/7" || p["cwd"] != "/repo" || p["mode"] != "worktree" {
				t.Errorf("prepare: %v", p)
			}
			return map[string]any{"branch": "feature", "worktreePath": "/repo-review"}
		case "orchestration.dispatchCommand":
			switch p["type"] {
			case "thread.create":
				threadID, _ = p["threadId"].(string)
				if p["worktreePath"] != "/repo-review" || p["projectId"] != "project" {
					t.Errorf("create: %v", p)
				}
			case "message.dispatch":
				messages = append(messages, p["text"].(string))
			default:
				t.Errorf("unexpected command: %v", p)
			}
		default:
			t.Errorf("unexpected RPC: %s", tag)
		}
		return map[string]any{}
	})
	links := &reviewTestLinks{}
	mux := http.NewServeMux()
	mountReviewAsk(mux, reviewTestService{links: links}, reviewTestMonitor{client}, reviewTestGithub{"CLOSED"})
	body := `{"launch":{"instance_id":"instance","project_id":"project","provider":"codex","model":"model"}}`
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/github/repos/o/r/pulls/7/review-thread", strings.NewReader(body)))
	if w.Code != 201 {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	if threadID == "" || len(links.values) != 1 {
		t.Fatalf("thread %q links %v", threadID, links.values)
	}
	link := links.values[0]
	if link.ThreadID != threadID || link.Kind != "review" || link.Repo != "o/r" || link.Number != 7 || link.InstanceID != "instance" {
		t.Fatalf("link: %+v", link)
	}
	if len(messages) != 1 || !strings.HasSuffix(messages[0], "Soy el usuario y te haré consultas sobre esta PR. No hagas nada todavía: no resumas ni analices; espera mi primera pregunta.") {
		t.Fatalf("messages: %v", messages)
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/github/repos/o/r/pulls/7/ask", strings.NewReader(`{"path":"a.go","question":"¿Por qué?"}`)))
	if w.Code != 200 || len(links.values) != 1 || len(messages) != 2 || !strings.Contains(messages[1], "¿Por qué?") {
		t.Fatalf("reuse: status %d links %v messages %v", w.Code, links.values, messages)
	}

}

func TestCleanReviewsKeepsChat(t *testing.T) {
	for _, state := range []string{"CLOSED", "MERGED", "OPEN"} {
		t.Run(state, func(t *testing.T) {
			removed := 0
			client := reviewRPC(t, func(tag string, p map[string]any) any {
				switch tag {
				case "server.getConfig":
					return map[string]any{}
				case "orchestration.getThreadProjection":
					return map[string]any{"thread": map[string]any{"projectId": "project", "worktreePath": "/repo-review"}}
				case "vcs.removeWorktree":
					removed++
					if p["cwd"] != "/repo" || p["path"] != "/repo-review" || p["force"] != false {
						t.Errorf("remove: %v", p)
					}
				default:
					t.Errorf("unexpected RPC: %s", tag)
				}
				return map[string]any{}
			})
			links := &reviewTestLinks{values: []service.IssueThread{{Kind: "review", Repo: "o/r", Number: 7, ThreadID: "thread", InstanceID: "instance"}}}
			svc := reviewTestService{links: links}
			for i := 0; i < 2; i++ {
				cleanReviews(context.Background(), svc, reviewTestMonitor{client}, reviewTestGithub{state})
			}
			want := 2
			if state == "OPEN" {
				want = 0
			}
			if removed != want || len(links.values) != 1 {
				t.Fatalf("removed %d, links %v", removed, links.values)
			}
		})
	}
}
