package peer

import (
	"context"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/node/localstore"
	"github.com/migueel15/dory/node/syncx"
)

var fast = Intervals{Heartbeat: 50 * time.Millisecond, Timeout: 200 * time.Millisecond,
	SeenWindow: 300 * time.Millisecond, BackoffMin: 20 * time.Millisecond, BackoffMax: 100 * time.Millisecond,
	Probe: time.Second}

type node struct {
	id    *Identity
	group *Log
	store *localstore.Store
	srv   *Server
	pres  *Presence
	addr  string
}

// pair builds two members and a shared membership (each node its own copy).
func pair(t *testing.T) (a, b *node) {
	t.Helper()
	ia, ib := ident(t, "a"), ident(t, "b")
	ga, _ := NewGroup(ia)
	if _, err := ga.Append(ia, ActionAdd, ib.PublicKey(), "b"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "g.json")
	if err := ga.SaveFile(path); err != nil {
		t.Fatal(err)
	}
	gb, _ := LoadLog(path)
	return startNode(t, ia, ga), startNode(t, ib, gb)
}

func startNode(t *testing.T, id *Identity, g *Log) *node {
	t.Helper()
	st, err := localstore.Open(filepath.Join(t.TempDir(), "n.db"), localstore.Options{Device: id.DeviceID(), Local: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	pres := NewPresence(fast)
	n := &node{id: id, group: g, store: st, pres: pres}
	n.srv = &Server{Identity: id, Members: g, Log: st.Log(), Presence: pres, Intervals: fast}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	n.addr = ln.Addr().String()
	go n.srv.Serve(ln)
	t.Cleanup(func() { n.srv.Close() })
	return n
}

func (n *node) put(t *testing.T, name string) {
	t.Helper()
	if _, err := n.store.Environments().Put(context.Background(), "t", service.Environment{Name: name}); err != nil {
		t.Fatal(err)
	}
}

func (n *node) names(t *testing.T) []string {
	t.Helper()
	l, err := n.store.Environments().List(context.Background(), service.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range l {
		out = append(out, e.Name)
	}
	return out
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timeout waiting for %s", what)
}

func has(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func TestSyncBothWaysOverMTLS(t *testing.T) {
	a, b := pair(t)
	a.put(t, "from-a")
	b.put(t, "from-b")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, p := range []struct{ from, to *node }{{a, b}, {b, a}} {
		cl, err := NewClient(p.from.id, p.from.group, p.to.addr, p.to.id.DeviceID(), fast, p.from.pres)
		if err != nil {
			t.Fatal(err)
		}
		eng := &syncx.Engine{Log: p.from.store.Log()}
		go eng.SyncWith(ctx, cl)
	}
	waitFor(t, "initial convergence", func() bool {
		return has(a.names(t), "from-b") && has(b.names(t), "from-a")
	})
	a.put(t, "live-a")
	b.put(t, "live-b")
	waitFor(t, "live ops", func() bool {
		return has(a.names(t), "live-b") && has(b.names(t), "live-a")
	})
	if st := a.pres.Status(b.id.DeviceID()); st.State != Connected {
		t.Fatalf("presence = %+v", st)
	}
	cl, _ := NewClient(a.id, a.group, b.addr, "", fast, nil)
	pb, err := cl.Presence(ctx)
	if err != nil || pb.DeviceID != b.id.DeviceID() {
		t.Fatalf("presence endpoint: %+v %v", pb, err)
	}
}

func TestSnapshotBootstrap(t *testing.T) {
	a, b := pair(t)
	a.put(t, "one")
	a.put(t, "two")
	cl, _ := NewClient(b.id, b.group, a.addr, a.id.DeviceID(), fast, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go (&syncx.Engine{Log: b.store.Log()}).SyncWith(ctx, cl)
	waitFor(t, "bootstrap", func() bool { return len(b.names(t)) == 2 })
}

func TestOutsiderRejected(t *testing.T) {
	a, _ := pair(t)
	ic := ident(t, "c")
	// c trusts a, so only the server side can refuse it.
	gc, _ := NewGroup(ic)
	gc.Append(ic, ActionAdd, a.id.PublicKey(), "a")
	cl, _ := NewClient(ic, gc, a.addr, a.id.DeviceID(), fast, nil)
	_, err := cl.Vector(context.Background())
	if err == nil || !strings.Contains(err.Error(), "tls") {
		t.Fatalf("outsider should get a TLS error, got %v", err)
	}
	// c does not trust a: the client refuses the server.
	gc2, _ := NewGroup(ic)
	cl2, _ := NewClient(ic, gc2, a.addr, "", fast, nil)
	if _, err = cl2.Vector(context.Background()); err == nil || !strings.Contains(err.Error(), "not a group member") {
		t.Fatalf("expected membership error, got %v", err)
	}
}

func TestPresenceDisconnectsAfterPeerStops(t *testing.T) {
	a, b := pair(t)
	cl, _ := NewClient(a.id, a.group, b.addr, b.id.DeviceID(), fast, a.pres)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go (&syncx.Engine{Log: a.store.Log()}).SyncWith(ctx, cl)
	bid := b.id.DeviceID()
	waitFor(t, "connected", func() bool { return a.pres.Status(bid).State == Connected })
	b.srv.Close()
	waitFor(t, "seen", func() bool { return a.pres.Status(bid).State == Seen })
	waitFor(t, "disconnected", func() bool { return a.pres.Status(bid).State == Disconnected })
}

func TestConnectorAndAddressRecord(t *testing.T) {
	a, b := pair(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mk := func(me *node, manual map[string][]string) *Connector {
		return &Connector{Identity: me.id, Group: me.group, Engine: &syncx.Engine{Log: me.store.Log()},
			Presence: me.pres, Book: me.store, Manual: manual, Intervals: fast, ScanEvery: 50 * time.Millisecond}
	}
	// b publishes its address and a learns it from a first manual sync.
	if err := b.store.PublishAddress(ctx, localstore.DeviceAddress{DeviceID: b.id.DeviceID(), Name: "b", Local: []string{b.addr}}); err != nil {
		t.Fatal(err)
	}
	a.put(t, "from-a")
	boot, bootCancel := context.WithCancel(ctx)
	// one dead and one live manual address: the live one must win.
	ca := mk(a, map[string][]string{b.id.DeviceID(): {"127.0.0.1:1", b.addr}})
	go ca.Run(boot)
	waitFor(t, "a learns b's record", func() bool {
		r, ok, _ := a.store.AddressOf(ctx, b.id.DeviceID())
		return ok && len(r.Local) == 1 && r.Local[0] == b.addr
	})
	bootCancel()
	// New connector without manual addresses: it uses only the record.
	cb2 := mk(a, nil)
	cands := cb2.Candidates(ctx, b.id.DeviceID())
	if len(cands) != 1 || cands[0].Source != "lan" || cands[0].Addr != b.addr {
		t.Fatalf("candidates = %+v", cands)
	}
	cl, cd, err := cb2.Dial(ctx, b.id.DeviceID())
	if err != nil || cd.Addr != b.addr || cl == nil {
		t.Fatalf("dial: %+v %v", cd, err)
	}
	go cb2.Run(ctx)
	// and a's later writes keep flowing to a once b also syncs from a.
	cb := mk(b, map[string][]string{a.id.DeviceID(): {a.addr}})
	go cb.Run(ctx)
	waitFor(t, "b pulls a", func() bool { return has(b.names(t), "from-a") })
	b.put(t, "from-b")
	waitFor(t, "b record flows", func() bool { return has(a.names(t), "from-b") })
}
