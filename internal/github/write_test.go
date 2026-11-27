package github

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReviewPullBlocksSelfApproval(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			_ = json.NewEncoder(w).Encode(map[string]string{"login": "alice"})
		case "/repos/org/demo/pulls/7":
			_ = json.NewEncoder(w).Encode(map[string]any{"number": 7, "user": map[string]string{"login": "alice"}})
		case "/repos/org/demo/pulls/7/reviews":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Setenv("DORY_GITHUB_TOKEN", "tok")
	c := New(nil, &Auth{})
	c.Base = srv.URL

	err := c.ReviewPull(t.Context(), "org/demo", 7, PullReviewInput{Event: "APPROVE", Body: "LGTM"})
	if err == nil || !strings.Contains(err.Error(), "own pull request") {
		t.Fatalf("expected self-approval error, got %v", err)
	}
	err = c.ReviewPull(t.Context(), "org/demo", 7, PullReviewInput{Event: "COMMENT", Body: "Note"})
	if err != nil {
		t.Fatalf("expected COMMENT to succeed on own PR, got %v", err)
	}
}

func TestCreateAndUpdateIssueAndPull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/repos/org/demo/issues":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number": 10, "title": "Bug", "body": "Desc", "state": "open", "html_url": "https://github.com/org/demo/issues/10",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/repos/org/demo/pulls":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number": 11, "title": "Fix", "state": "open", "draft": true, "head": map[string]string{"ref": "feat/fix"}, "base": map[string]string{"ref": "main"},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Setenv("DORY_GITHUB_TOKEN", "tok")
	c := New(nil, &Auth{})
	c.Base = srv.URL

	iss, body, err := c.CreateIssue(t.Context(), "org/demo", IssueCreateInput{Title: "Bug", Body: "Desc"})
	if err != nil || iss.Number != 10 || body != "Desc" {
		t.Fatalf("CreateIssue failed: %+v %q %v", iss, body, err)
	}
	pr, err := c.CreatePull(t.Context(), "org/demo", PullCreateInput{Title: "Fix", Head: "feat/fix", Base: "main", Draft: true})
	if err != nil || pr.Number != 11 || !pr.Draft {
		t.Fatalf("CreatePull failed: %+v %v", pr, err)
	}
}
