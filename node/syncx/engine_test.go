package syncx_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/service"
	"github.com/migueel15/dory/node/localstore"
	"github.com/migueel15/dory/node/syncx"
)

type node struct {
	t     *testing.T
	store *localstore.Store
	eng   *syncx.Engine
}

func newNode(t *testing.T, device string) *node {
	t.Helper()
	s, err := localstore.Open(filepath.Join(t.TempDir(), device+".db"), localstore.Options{Device: device, Local: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return &node{t: t, store: s, eng: &syncx.Engine{Log: s.Log(), Batch: 7}}
}

func (n *node) project(slug string) {
	n.t.Helper()
	if _, err := n.store.Projects().Put(context.Background(), "u", service.Project{Slug: slug, Name: slug}); err != nil {
		n.t.Fatal(err)
	}
}

func (n *node) vector() syncx.VersionVector {
	n.t.Helper()
	vv, err := n.store.Log().Vector(context.Background())
	if err != nil {
		n.t.Fatal(err)
	}
	return vv
}

func (n *node) slugs() []string {
	n.t.Helper()
	ps, err := n.store.Projects().List(context.Background(), service.Filter{Limit: 1000})
	if err != nil {
		n.t.Fatal(err)
	}
	var out []string
	for _, p := range ps {
		out = append(out, p.Slug)
	}
	return out
}

// link is a one way replication from -> into with its own transport.
type link struct {
	tr   *syncx.MemTransport
	done chan error
	stop context.CancelFunc
}

func (n *node) follow(from *node) *link {
	tr := syncx.NewMemTransport(from.store.Log())
	return n.followVia(tr)
}

func (n *node) followVia(tr *syncx.MemTransport) *link {
	ctx, cancel := context.WithCancel(context.Background())
	l := &link{tr: tr, done: make(chan error, 1), stop: cancel}
	go func() { l.done <- n.eng.SyncWith(ctx, tr) }()
	return l
}

func (l *link) close() { l.stop(); <-l.done }

func waitFor(t *testing.T, d time.Duration, what string, ok func() bool) {
	t.Helper()
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		if ok() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func sameVector(a, b *node) func() bool {
	return func() bool { return reflect.DeepEqual(a.vector(), b.vector()) && len(a.vector()) > 0 }
}

func TestBridgeCutReconnectAndBootstrap(t *testing.T) {
	a, b, c := newNode(t, "a"), newNode(t, "b"), newNode(t, "c")
	for i := 0; i < 20; i++ {
		a.project(fmt.Sprintf("a-%d", i))
	}
	b.project("b-0")
	c.project("c-0")

	// A and C never connect; B bridges both ways.
	ab, ba := a.follow(b), b.follow(a)
	bc, cb := b.follow(c), c.follow(b)
	defer func() { ab.close(); ba.close(); bc.close(); cb.close() }()

	waitFor(t, 5*time.Second, "initial convergence", func() bool {
		return sameVector(a, b)() && sameVector(b, c)()
	})
	if len(c.slugs()) != 22 {
		t.Fatalf("c has %v", c.slugs())
	}

	// Live latency in memory, from A through B to C.
	start := time.Now()
	a.project("live")
	waitFor(t, time.Second, "live op at c", func() bool { return len(c.slugs()) == 23 })
	if d := time.Since(start); d > 200*time.Millisecond {
		t.Fatalf("live latency %v", d)
	}

	// Cut B-C, edit on both sides, reconnect: nothing lost, nothing duplicated.
	bc.tr.Cut()
	cb.tr.Cut()
	if err := <-bc.done; !errors.Is(err, syncx.ErrDisconnected) {
		t.Fatalf("expected disconnect, got %v", err)
	}
	if err := <-cb.done; !errors.Is(err, syncx.ErrDisconnected) {
		t.Fatalf("expected disconnect, got %v", err)
	}
	for i := 0; i < 10; i++ {
		a.project(fmt.Sprintf("offline-a-%d", i))
		c.project(fmt.Sprintf("offline-c-%d", i))
	}
	time.Sleep(50 * time.Millisecond)
	if len(c.slugs()) != 33 {
		t.Fatalf("c leaked while cut: %d", len(c.slugs()))
	}
	bc.tr.Restore()
	cb.tr.Restore()
	bc = b.followVia(bc.tr)
	cb = c.followVia(cb.tr)

	waitFor(t, 5*time.Second, "reconnect convergence", func() bool {
		return sameVector(a, b)() && sameVector(b, c)() && len(a.slugs()) == 43
	})
	for _, n := range []*node{a, b, c} {
		if got := len(n.slugs()); got != 43 {
			t.Fatalf("expected 43 projects, got %d", got)
		}
		var ops, distinct int
		n.store.Log().Vector(context.Background())
		all, _ := n.store.Log().Since(context.Background(), nil, 1_000_000)
		seen := map[string]bool{}
		for _, o := range all {
			ops++
			seen[fmt.Sprintf("%s/%d", o.DeviceID, o.Seq)] = true
		}
		distinct = len(seen)
		if ops != distinct {
			t.Fatalf("duplicate ops: %d vs %d", ops, distinct)
		}
	}
	if !reflect.DeepEqual(a.vector(), c.vector()) {
		t.Fatalf("vectors differ: %v %v", a.vector(), c.vector())
	}

	// A new node bootstraps from a snapshot of B, then follows live.
	d := newNode(t, "d")
	var changed int
	var mu sync.Mutex
	d.eng.OnChange = func(ops []syncx.Op) { mu.Lock(); changed += len(ops); mu.Unlock() }
	db := d.follow(b)
	defer db.close()
	waitFor(t, 5*time.Second, "bootstrap", sameVector(d, b))
	if got := len(d.slugs()); got != 43 {
		t.Fatalf("bootstrap got %d", got)
	}
	c.project("after-bootstrap")
	waitFor(t, time.Second, "live after bootstrap", func() bool { return len(d.slugs()) == 44 })
	mu.Lock()
	if changed == 0 {
		t.Fatal("OnChange never called")
	}
	mu.Unlock()

	// Remote audit events land in the events table the SSE stream polls.
	evs, err := d.store.ListEvents(context.Background(), 0, "", 1000)
	if err != nil || len(evs) < 44 {
		t.Fatalf("events at d: %d %v", len(evs), err)
	}
}
