package peer

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/migueel15/dory/node/localstore"
	"github.com/migueel15/dory/node/syncx"
)

// host starts A alone in its own group with a pairing endpoint.
func host(t *testing.T, confirm func(string, string) bool) (*node, *Pairing) {
	t.Helper()
	ia := ident(t, "a")
	g, _ := NewGroup(ia)
	n := &node{id: ia, group: g}
	st, err := localstore.Open(filepath.Join(t.TempDir(), "a.db"), localstore.Options{Device: ia.DeviceID(), Local: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	n.store = st
	n.pres = NewPresence(fast)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	n.addr = ln.Addr().String()
	p := &Pairing{Identity: ia, Group: g, Data: st.Log(), Addrs: []string{n.addr}, Confirm: confirm}
	n.srv = &Server{Identity: ia, Members: g, Log: st.Log(), Presence: n.pres, Intervals: fast, Pairing: p}
	go n.srv.Serve(ln)
	t.Cleanup(func() { n.srv.Close() })
	return n, p
}

func yes(string, string) bool { return true }

func TestPairingFull(t *testing.T) {
	var asked atomic.Value
	a, p := host(t, func(name, key string) bool { asked.Store(name + "|" + key); return true })
	a.put(t, "from-a")
	offer, err := p.Start(time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if png, err := offer.PNG(256); err != nil || len(png) < 100 {
		t.Fatalf("png: %v", err)
	}
	ib := ident(t, "b")
	res, err := Join(context.Background(), offer.Payload, ib, "phone")
	if err != nil {
		t.Fatal(err)
	}
	if got := asked.Load(); got != "phone|"+ib.PublicKeyHex() {
		t.Fatalf("confirm saw %v", got)
	}
	if !res.Group.Allowed(ib.PublicKey()) || !a.group.Allowed(ib.PublicKey()) {
		t.Fatal("b not a member on both sides")
	}
	if p.Open() {
		t.Fatal("window should close after success")
	}
	bn := startNode(t, ib, res.Group)
	if err := res.ApplySnapshot(context.Background(), bn.store.Log()); err != nil {
		t.Fatal(err)
	}
	if !has(bn.names(t), "from-a") {
		t.Fatalf("b lacks data: %v", bn.names(t))
	}
	// normal sync both ways afterwards
	a.put(t, "live-a")
	bn.put(t, "live-b")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, x := range []struct{ from, to *node }{{a, bn}, {bn, a}} {
		cl, err := NewClient(x.from.id, x.from.group, x.to.addr, x.to.id.DeviceID(), fast, nil)
		if err != nil {
			t.Fatal(err)
		}
		go (&syncx.Engine{Log: x.from.store.Log()}).SyncWith(ctx, cl)
	}
	waitFor(t, "sync", func() bool { return has(a.names(t), "live-b") && has(bn.names(t), "live-a") })
}

func TestPairingRoutesStayMembersOnly(t *testing.T) {
	a, p := host(t, yes)
	p.Start(time.Minute)
	ib := ident(t, "b")
	cl, _ := NewClient(ib, a.group, a.addr, "", fast, nil)
	if _, err := cl.Vector(context.Background()); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("non-member read vector: %v", err)
	}
	p.Close()
	if _, err := cl.Vector(context.Background()); err == nil {
		t.Fatal("handshake should fail with window closed")
	}
}

func jsonQR(q QRPayload) (string, error) {
	b, err := json.Marshal(q)
	return string(b), err
}

func TestPairingFailures(t *testing.T) {
	ctx := context.Background()
	t.Run("expired", func(t *testing.T) {
		_, p := host(t, yes)
		offer, _ := p.Start(50 * time.Millisecond)
		time.Sleep(80 * time.Millisecond)
		if _, err := Join(ctx, offer.Payload, ident(t, "b"), "b"); err == nil {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("reused", func(t *testing.T) {
		a, p := host(t, yes)
		offer, _ := p.Start(time.Minute)
		if _, err := Join(ctx, offer.Payload, ident(t, "b"), "b"); err != nil {
			t.Fatal(err)
		}
		before := len(a.group.Members())
		if _, err := Join(ctx, offer.Payload, ident(t, "c"), "c"); err == nil {
			t.Fatal("token reused")
		}
		if len(a.group.Members()) != before {
			t.Fatal("member added with reused token")
		}
		// even if the host reopens a window, the old token is dead
		p.Start(time.Minute)
		if _, err := Join(ctx, offer.Payload, ident(t, "d"), "d"); err == nil {
			t.Fatal("old token accepted in new window")
		}
	})
	t.Run("key mismatch", func(t *testing.T) {
		_, p := host(t, yes)
		offer, _ := p.Start(time.Minute)
		q, _ := ParseQR(offer.Payload)
		q.APubKey = ident(t, "other").PublicKeyHex()
		bad, _ := jsonQR(q)
		_, err := Join(ctx, bad, ident(t, "b"), "b")
		if err == nil || !strings.Contains(err.Error(), ErrKeyMismatch.Error()) {
			t.Fatalf("err = %v", err)
		}
		if !p.Open() {
			t.Fatal("pinning failure must not consume the token")
		}
	})
	t.Run("rejected", func(t *testing.T) {
		a, p := host(t, func(string, string) bool { return false })
		offer, _ := p.Start(time.Minute)
		ib := ident(t, "b")
		if _, err := Join(ctx, offer.Payload, ib, "b"); err == nil || !strings.Contains(err.Error(), "rejected") {
			t.Fatalf("err = %v", err)
		}
		if a.group.Allowed(ib.PublicKey()) || p.Open() {
			t.Fatal("rejected device added or window left open")
		}
	})
	t.Run("three failures close", func(t *testing.T) {
		_, p := host(t, yes)
		offer, _ := p.Start(time.Minute)
		q, _ := ParseQR(offer.Payload)
		good := q.Token
		q.Token = strings.Repeat("0", 32)
		bad, _ := jsonQR(q)
		for i := 0; i < 3; i++ {
			if _, err := Join(ctx, bad, ident(t, "b"), "b"); err == nil {
				t.Fatal("bad token accepted")
			}
		}
		if p.Open() {
			t.Fatal("window still open")
		}
		q.Token = good
		ok, _ := jsonQR(q)
		if _, err := Join(ctx, ok, ident(t, "b"), "b"); err == nil {
			t.Fatal("good token accepted after window closed")
		}
	})
}

func TestEnsureGroup(t *testing.T) {
	dir := t.TempDir()
	id := ident(t, "a")
	g, created, err := EnsureGroup(dir, id)
	if err != nil || !created || !g.Allowed(id.PublicKey()) {
		t.Fatalf("%v %v", created, err)
	}
	g2, created, err := EnsureGroup(dir, id)
	if err != nil || created || g2.GroupID != g.GroupID {
		t.Fatalf("%v %v", created, err)
	}
}
