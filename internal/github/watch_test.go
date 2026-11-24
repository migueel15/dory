package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFinalState(t *testing.T) {
	cases := []struct {
		state         string
		draft, merged bool
		want          string
	}{
		{"open", false, false, "open"},
		{"open", true, false, "draft"},
		{"closed", false, true, "merged"},
		{"closed", false, false, "closed"},
	}
	for _, c := range cases {
		if got := finalState(c.state, c.draft, c.merged); got != c.want {
			t.Errorf("%+v: got %s", c, got)
		}
	}
}

func TestPollRepoETag(t *testing.T) {
	body := `[{"number":7,"state":"open","draft":true,"updated_at":"a","head":{"ref":"feat/x"}}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Write([]byte(body))
	}))
	defer srv.Close()
	c := New(nil, NewAuth(t.TempDir()+"/token", func(context.Context) string { return "t" }))
	c.Base = srv.URL
	ctx := context.Background()
	if changed, _, err := c.pollRepo(ctx, "o/r"); err != nil || changed {
		t.Fatalf("first poll: %v %v", changed, err)
	}
	if changed, _, err := c.pollRepo(ctx, "o/r"); err != nil || changed {
		t.Fatalf("304 poll: %v %v", changed, err)
	}
	c.watched["o/r"] = watchEntry{print: c.watched["o/r"].print}
	body = `[{"number":7,"state":"open","draft":false,"updated_at":"b","head":{"ref":"feat/x"}}]`
	if changed, _, err := c.pollRepo(ctx, "o/r"); err != nil || !changed {
		t.Fatalf("changed poll: %v %v", changed, err)
	}
	if e, _ := states.Load("o/r#feat/x"); e.(stateEntry).ref.State != "open" {
		t.Fatalf("branch state: %+v", e)
	}
}

func TestHasPendingCIAndFingerprint(t *testing.T) {
	idle := []Pull{{Number: 1, CI: "SUCCESS", CIPassed: 3}}
	if hasPendingCI(idle) {
		t.Fatal("expected no pending CI")
	}
	running := []Pull{{Number: 1, CI: "PENDING", CIPassed: 2, CIPending: 1}}
	if !hasPendingCI(running) {
		t.Fatal("expected pending CI")
	}
	if ciFingerprint(idle) == ciFingerprint(running) {
		t.Fatal("expected different CI fingerprints")
	}
}
