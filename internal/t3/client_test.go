package t3

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func server(t *testing.T, protocol, shell string) Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/t3/environment", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"orchestrationProtocolVersion":` + protocol + `}`))
	})
	mux.HandleFunc("/api/orchestration/shell", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(shell))
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return Client{BaseURL: s.URL, Token: "good", HTTP: s.Client()}
}

func TestThreadsDropsSettledAndFlagsAttention(t *testing.T) {
	c := server(t, "2", `{"threads":[
		{"id":"a","status":"running","pendingRuntimeRequest":null},
		{"id":"b","status":"running","pendingRuntimeRequest":{"kind":"x"}},
		{"id":"c","status":"failed","settledAt":"2026-01-01"},
		{"id":"d","status":"waiting","snoozedUntil":"2099-01-01"}]}`)
	got, err := c.Threads(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"a": false, "b": true, "d": false}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for _, th := range got {
		if want[th.ID] != th.Attention {
			t.Errorf("%s attention=%v", th.ID, th.Attention)
		}
	}
}

func TestRejections(t *testing.T) {
	if _, err := server(t, "3", "{}").Discover(context.Background()); !errors.Is(err, errIncompatible) {
		t.Errorf("protocol: %v", err)
	}
	c := server(t, "2", "{}")
	c.Token = "bad"
	if _, err := c.Threads(context.Background()); !errors.Is(err, errUnauthorized) {
		t.Errorf("token: %v", err)
	}
}

func TestHTMLFallbackIsAnError(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html>"))
	}))
	defer s.Close()
	if _, err := (Client{BaseURL: s.URL, HTTP: s.Client()}).Discover(context.Background()); err == nil {
		t.Error("HTML accepted as discovery")
	}
}

func TestShellKeepsEveryThreadWithLineage(t *testing.T) {
	c := server(t, "2", `{"threads":[
		{"id":"a","status":"running"},
		{"id":"s","status":"completed","settledAt":"2026-01-01","pinnedAt":"2026-01-02","pinOrderKey":"a0"},
		{"id":"k","status":"completed","lineage":{"parentThreadId":"a","relationshipToParent":"subagent"}},
		{"id":"w","status":"running","lineage":{"parentThreadId":"a","relationshipToParent":"subagent"}},
		{"id":"x","status":"completed","deletedAt":"2026-01-03"}],
		"archivedThreads":[{"id":"z","status":"completed","archivedAt":"2026-01-04"}]}`)
	active, all, err := c.shell(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ids := func(l []Thread) (out []string) {
		for _, th := range l {
			out = append(out, th.ID)
		}
		return
	}
	if got := ids(active); len(got) != 2 || got[0] != "a" || got[1] != "w" {
		t.Errorf("active = %v", got)
	}
	if got := ids(all); len(got) != 5 {
		t.Errorf("all = %v", got)
	}
	for _, th := range all {
		switch th.ID {
		case "s":
			if th.PinnedAt == "" || th.PinOrderKey != "a0" || th.SettledAt == "" {
				t.Errorf("s = %+v", th)
			}
		case "k":
			if th.ParentID != "a" || th.Relation != "subagent" {
				t.Errorf("k = %+v", th)
			}
		case "z":
			if th.ArchivedAt == "" {
				t.Errorf("z = %+v", th)
			}
		}
	}
}
