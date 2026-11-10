package api

import (
	"context"
	"github.com/migueel15/dory/internal/service"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/github"
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
	if f.Fields["kind"] != "review" || f.Fields["repo"] != "o/r" || f.Fields["number"] != 7 {
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
