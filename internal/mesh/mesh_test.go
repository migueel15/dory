package mesh

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/oplog"
)

type node struct {
	m     *Mesh
	store *data.Store
}

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func start(t *testing.T, ctx context.Context, name string) node {
	t.Helper()
	dir := t.TempDir()
	db, err := localdb.Open(ctx, filepath.Join(dir, "dory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	log, err := oplog.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	id, err := LoadIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := New(db, log, id, name, freePort(t))
	log.OnChange = func(string, string) { m.Changed() }
	m.Info = func(context.Context) map[string]any { return map[string]any{"name": name} }
	m.API = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, "%s answered %s", name, r.URL.Path) })
	go m.Serve(ctx)
	// Wait until the listener is up.
	for range 100 {
		if c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", m.port)); err == nil {
			c.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return node{m, data.New(db, log)}
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for range 300 {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestPairSyncAndForward(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := start(t, ctx, "portatil"), start(t, ctx, "servidor")

	if _, err := b.m.Pair(ctx, fmt.Sprintf("127.0.0.1:%d", a.m.port), "0000-0000"); err == nil {
		t.Fatal("paired without an open code")
	}
	code, _ := a.m.PairCode()
	if _, err := b.m.Pair(ctx, fmt.Sprintf("127.0.0.1:%d", a.m.port), "1111-1111"); err == nil {
		t.Fatal("paired with a wrong code")
	}
	p, err := b.m.Pair(ctx, fmt.Sprintf("127.0.0.1:%d", a.m.port), code)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "portatil" {
		t.Fatalf("paired with %+v", p)
	}
	// a learned b's address from the request: it must point at b's mesh port.
	peersA, _ := a.m.Peers(ctx)
	if len(peersA) != 1 || peersA[0].Address != fmt.Sprintf("127.0.0.1:%d", b.m.port) {
		t.Fatalf("a's view of b: %+v", peersA)
	}

	// A change on either side reaches the other.
	project, err := a.store.Projects().Put(ctx, data.Project{Slug: "dory", Name: "Dory"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "project on b", func() bool { _, err := b.store.Projects().Get(ctx, project.ID); return err == nil })
	task, _ := b.store.Tasks().Put(ctx, data.Task{ProjectID: &project.ID, Title: "Desde el servidor"})
	eventually(t, "task on a", func() bool { _, err := a.store.Tasks().Get(ctx, task.ID); return err == nil })

	// Presence: each side sees the other connected.
	eventually(t, "presence", func() bool {
		pa, _ := a.m.Peers(ctx)
		pb, _ := b.m.Peers(ctx)
		return len(pa) == 1 && pa[0].Connected && pa[0].Protocol == ProtocolVersion &&
			len(pb) == 1 && pb[0].Connected && pb[0].Protocol == ProtocolVersion
	})

	// A call for a thread of a is answered by a.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/threads/x", nil)
	if !b.m.Forward(rec, req, a.m.log.Node()) {
		t.Fatal("forward failed")
	}
	body, _ := io.ReadAll(rec.Body)
	if string(body) != "portatil answered /api/threads/x" {
		t.Fatalf("forwarded answer %q", body)
	}

	// Revoked, the server no longer syncs with the laptop.
	if err = a.m.Revoke(ctx, b.m.log.Node()); err != nil {
		t.Fatal(err)
	}
	late, _ := b.store.Tasks().Put(ctx, data.Task{Title: "Tras revocar"})
	time.Sleep(500 * time.Millisecond)
	if _, err := a.store.Tasks().Get(ctx, late.ID); err == nil {
		t.Fatal("revoked node still syncs")
	}
}
