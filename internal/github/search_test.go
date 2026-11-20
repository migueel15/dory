package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchScopesStatesAndPartialErrors(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if !strings.Contains(q, "repo:owner/repo") || strings.Contains(q, "repo:evil") {
			t.Errorf("unsafe scope: %q", q)
		}
		if strings.Contains(q, "is:open") {
			fmt.Fprintf(w, `{"items":[{"repository_url":%q,"number":1,"title":"draft","state":"open","draft":true,"pull_request":{}},{"repository_url":%q,"number":9,"title":"foreign","state":"open"}],"incomplete_results":false}`, server.URL+"/repos/owner/repo", server.URL+"/repos/evil/repo")
		} else {
			fmt.Fprintf(w, `{"items":[{"repository_url":%q,"number":2,"title":"merged","state":"closed","pull_request":{"merged_at":"2026-01-01"}}],"incomplete_results":true}`, server.URL+"/repos/owner/repo")
		}
	}))
	defer server.Close()
	c := New(func(context.Context) (map[string]string, error) {
		return map[string]string{"https://github.com/owner/repo.git": "p"}, nil
	}, NewAuth(t.TempDir()+"/token", func(context.Context) string { return "t" }))
	c.Base = server.URL
	c.HTTP = server.Client()
	got, err := c.Search(context.Background(), `hola repo:evil`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 2 || got.Items[0].State != "draft" || got.Items[1].State != "merged" || !got.Incomplete {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestSearchNumberAndFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/a/one/issues/12":
			fmt.Fprint(w, `{"number":12,"title":"issue","state":"open"}`)
		case "/repos/b/two/issues/12":
			http.Error(w, "unavailable", http.StatusTooManyRequests)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := New(func(context.Context) (map[string]string, error) {
		return map[string]string{"https://github.com/a/one": "p", "https://github.com/b/two": "p"}, nil
	}, NewAuth(t.TempDir()+"/token", func(context.Context) string { return "t" }))
	c.Base = server.URL
	c.HTTP = server.Client()
	got, err := c.Search(context.Background(), "#12")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].Kind != "issues" || !got.Incomplete {
		t.Fatalf("unexpected result: %+v", got)
	}
}
