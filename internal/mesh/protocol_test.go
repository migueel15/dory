package mesh

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testPeer(t *testing.T, h http.HandlerFunc) (address, key string) {
	t.Helper()
	id, err := LoadIdentity(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	peer := httptest.NewUnstartedServer(h)
	peer.TLS = &tls.Config{Certificates: []tls.Certificate{id.Cert}, MinVersion: tls.VersionTLS13}
	peer.StartTLS()
	t.Cleanup(peer.Close)
	return peer.Listener.Addr().String(), id.PublicKey()
}

func TestSyncRejectsIncompatibleProtocol(t *testing.T) {
	for _, tc := range []struct {
		name    string
		serve   http.HandlerFunc
		version int
	}{
		{"v1", http.NotFound, 1},
		{"v1-web", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, "<html></html>")
		}, 1},
		{"future", func(w http.ResponseWriter, _ *http.Request) {
			reply(w, map[string]any{"protocol": 3, "threads": "future-format"})
		}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			local := start(t, ctx, "local").m
			address, key := testPeer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/peer/v2/info" {
					t.Errorf("incompatible peer received %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				tc.serve(w, r)
			})
			if err := local.addPeer(ctx, "other", "Other", key, address); err != nil {
				t.Fatal(err)
			}
			if _, err := local.sync(ctx, "other"); err != nil {
				t.Fatal(err)
			}
			peers, err := local.Peers(ctx)
			if err != nil || len(peers) != 1 || !peers[0].Incompatible || peers[0].Connected || peers[0].Protocol != tc.version {
				t.Fatalf("peer status = %+v, %v", peers, err)
			}
			var lastError string
			if err := local.db.QueryRowContext(ctx, "SELECT last_error FROM peers WHERE node_id = 'other'").Scan(&lastError); err != nil {
				t.Fatal(err)
			}
			if want := fmt.Sprintf("incompatible_protocol: local=2 remote=%d", tc.version); lastError != want {
				t.Fatalf("last_error = %q, want %q", lastError, want)
			}
		})
	}
}

func TestPeerTransportReuseAndInvalidation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := start(t, ctx, "local").m
	add := func() {
		t.Helper()
		if err := m.addPeer(ctx, "other", "Other", "key", "localhost:7710"); err != nil {
			t.Fatal(err)
		}
	}
	transport := func() *http.Transport {
		t.Helper()
		p, err := m.peer(ctx, "other")
		if err != nil || p.transport == nil {
			t.Fatalf("peer = %+v, %v", p, err)
		}
		return p.transport
	}
	add()
	initial := transport()
	if transport() != initial {
		t.Fatal("a second request did not reuse the transport")
	}
	m.spotted(ctx, "other", "Other", "localhost:7711")
	changed := transport()
	if changed == initial {
		t.Fatal("address change reused the previous transport")
	}
	if err := m.SetEnabled(ctx, "other", false); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.transports["other"]; ok {
		t.Fatal("paused peer retained its transport")
	}
	add()
	if transport() == changed {
		t.Fatal("pair after pause reused the previous transport")
	}
	if err := m.Revoke(ctx, "other"); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.transports["other"]; ok {
		t.Fatal("revoked peer retained its transport")
	}
}

func TestForwardFailureAndWrittenResponse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := start(t, ctx, "local").m
	for _, written := range []bool{false, true} {
		t.Run(fmt.Sprintf("written=%v", written), func(t *testing.T) {
			address, key := testPeer(t, func(w http.ResponseWriter, _ *http.Request) {
				if written {
					w.WriteHeader(http.StatusEarlyHints)
				}
				panic(http.ErrAbortHandler)
			})
			if err := m.addPeer(ctx, "other", "Other", key, address); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/threads", strings.NewReader("{}"))
			rec := httptest.NewRecorder()
			if handled := m.Forward(rec, req, "other"); handled != written {
				t.Fatalf("Forward = %v, want %v", handled, written)
			}
			if written && rec.Code != http.StatusEarlyHints {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusEarlyHints)
			}
		})
	}
}
