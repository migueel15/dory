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
