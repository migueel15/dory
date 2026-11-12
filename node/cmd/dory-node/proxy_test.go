package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/migueel15/dory/node/peer"
)

var fast = peer.Intervals{Heartbeat: 50 * time.Millisecond, Timeout: 200 * time.Millisecond,
	SeenWindow: 300 * time.Millisecond, BackoffMin: 20 * time.Millisecond, BackoffMax: 100 * time.Millisecond, Probe: time.Second}

func eventually(t *testing.T, what string, f func() bool) {
	t.Helper()
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if f() {
			return
		}
	}
	t.Fatalf("timeout: %s", what)
}

func TestT3ForwardedToOwner(t *testing.T) {
	dA, dB := t.TempDir(), t.TempDir()
	idA, err := peer.LoadOrCreate(dA, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	idB, err := peer.LoadOrCreate(dB, "beta")
	if err != nil {
		t.Fatal(err)
	}
	g, _ := peer.NewGroup(idA)
	if _, err := g.Append(idA, peer.ActionAdd, idB.PublicKey(), "beta"); err != nil {
		t.Fatal(err)
	}
	peer.SaveGroup(dA, g)
	peer.SaveGroup(dB, g)

	release := make(chan struct{})
	var gotCookie, gotAuth string
	stub := http.NewServeMux()
	stub.HandleFunc("GET /api/t3/threads", func(w http.ResponseWriter, r *http.Request) {
		gotCookie, gotAuth = r.Header.Get("Cookie"), r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"id":"th-1","title":"from A"}]`))
	})
	stub.HandleFunc("GET /api/t3/threads/{id}/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.Write([]byte("event: snapshot\ndata: {\"n\":1}\n\n"))
		w.(http.Flusher).Flush()
		select {
		case <-release:
			w.Write([]byte("event: update\ndata: {\"n\":2}\n\n"))
			w.(http.Flusher).Flush()
		case <-r.Context().Done():
		}
	})

	a, err := Start(context.Background(), Options{DataDir: dA, T3URL: "http://127.0.0.1:1", Listen: "127.0.0.1:0", NoMDNS: true, Headless: true, Intervals: fast, t3Stub: stub, DeviceName: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			a.Stop()
		}
	})
	port := a.PeerAddr[strings.LastIndex(a.PeerAddr, ":")+1:]
	b, err := Start(context.Background(), Options{DataDir: dB, Listen: "127.0.0.1:0", NoMDNS: true, Headless: true, Intervals: fast, DeviceName: "beta",
		Manual: map[string][]string{idA.DeviceID(): {"127.0.0.1:" + port}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Stop)

	eventually(t, "B sees A's T3 instance and link", func() bool {
		o, _ := t3Owner(context.Background(), b.Store, idB.DeviceID())
		return o == idA.DeviceID() && b.Presence.Status(o).State == peer.Connected
	})
	tok, _ := os.ReadFile(filepath.Join(dB, "local-token"))
	token := strings.TrimSpace(string(tok))
	base := "http://" + b.HTTPAddr

	// No credentials: B's own auth still applies.
	if code, _ := do(t, &http.Client{}, "GET", base+"/api/t3/threads", "", nil); code != 401 {
		t.Fatalf("no token: %d", code)
	}
	code, body := do(t, &http.Client{}, "GET", base+"/api/t3/threads", token, nil)
	if code != 200 || !strings.Contains(string(body), "from A") {
		t.Fatalf("list via B: %d %s", code, body)
	}
	if gotAuth != "" || !strings.Contains(gotCookie, sessionCookie+"=") {
		t.Fatalf("owner saw auth %q cookie %q", gotAuth, gotCookie)
	}

	// SSE flushes: first event arrives while the stream is still open.
	req, _ := http.NewRequest("GET", base+"/api/t3/threads/th-1/stream", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	rd := bufio.NewReader(resp.Body)
	readEvent := func() string {
		var sb strings.Builder
		for {
			l, err := rd.ReadString('\n')
			if err != nil {
				t.Fatalf("stream: %v", err)
			}
			if l == "\n" {
				return sb.String()
			}
			sb.WriteString(l)
		}
	}
	if ev := readEvent(); !strings.Contains(ev, "snapshot") {
		t.Fatalf("first event: %q", ev)
	}
	close(release)
	if ev := readEvent(); !strings.Contains(ev, `"n":2`) {
		t.Fatalf("second event: %q", ev)
	}
	resp.Body.Close()

	// Owner stopped: 503 with the SPA error shape.
	a.Stop()
	stopped = true
	eventually(t, "A disconnected", func() bool { return b.Presence.Status(idA.DeviceID()).State != peer.Connected })
	code, body = do(t, &http.Client{}, "GET", base+"/api/t3/threads", token, nil)
	var e struct {
		Error struct{ Code, Message string }
	}
	if json.Unmarshal(body, &e) != nil || code != 503 || e.Error.Message != "alpha desconectado" {
		t.Fatalf("after stop: %d %s", code, body)
	}
}
