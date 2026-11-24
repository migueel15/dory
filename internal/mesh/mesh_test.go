package mesh

import (
	"context"
	"encoding/json"
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
	"github.com/migueel15/dory/internal/schema"
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
	project, err := a.store.Projects().Create(ctx, data.Project{Slug: "dory", Name: "Dory"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "project on b", func() bool { _, err := b.store.Projects().Get(ctx, project.ID); return err == nil })
	task, _ := b.store.Tasks().Create(ctx, data.Task{ProjectID: &project.ID, Title: "Desde el servidor"})
	eventually(t, "task on a", func() bool { _, err := a.store.Tasks().Get(ctx, task.ID); return err == nil })

	// Presence: each side sees the other connected.
	eventually(t, "presence", func() bool {
		pa, _ := a.m.Peers(ctx)
		pb, _ := b.m.Peers(ctx)
		return len(pa) == 1 && pa[0].Connected && pa[0].Protocol == ProtocolVersion &&
			len(pb) == 1 && pb[0].Connected && pb[0].Protocol == ProtocolVersion
	})

	peers, err := a.m.Peers(ctx)
	if err != nil || len(peers) != 1 {
		t.Fatalf("peers = %+v, %v", peers, err)
	}
	var info struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(peers[0].Info, &info); err != nil || info.Schema != schema.Hash() {
		t.Fatalf("peer schema = %q, want %q, decode error: %v", info.Schema, schema.Hash(), err)
	}

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
	late, _ := b.store.Tasks().Create(ctx, data.Task{Title: "Tras revocar"})
	time.Sleep(500 * time.Millisecond)
	if _, err := a.store.Tasks().Get(ctx, late.ID); err == nil {
		t.Fatal("revoked node still syncs")
	}
}

func TestMeshSharesThreadState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := start(t, ctx, "owner"), start(t, ctx, "observer")
	thread, err := a.store.Threads().Create(ctx, data.Thread{NodeID: a.m.log.Node(), Provider: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	archived, err := a.store.Threads().Create(ctx, data.Thread{NodeID: a.m.log.Node(), Provider: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.Threads().Archive(ctx, archived.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.store.SetThreadState(ctx, thread.ID, "working", ""); err != nil {
		t.Fatal(err)
	}
	own, err := b.store.Threads().Create(ctx, data.Thread{NodeID: b.m.log.Node(), Provider: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	code, _ := a.m.PairCode()
	if _, err := b.m.Pair(ctx, fmt.Sprintf("127.0.0.1:%d", a.m.port), code); err != nil {
		t.Fatal(err)
	}
	eventually(t, "owner state on observer", func() bool {
		got, err := b.store.Threads().Get(ctx, thread.ID)
		return err == nil && got.Status == "working" && !got.StateStale && got.Unread
	})
	if err := a.store.SetThreadState(ctx, thread.ID, "error", "provider failed"); err != nil {
		t.Fatal(err)
	}
	a.m.ThreadStateChanged()
	eventually(t, "changed owner state on observer", func() bool {
		var status, failure string
		err := b.m.db.QueryRowContext(ctx, `SELECT status, error FROM thread_state WHERE thread_id = ?`, thread.ID).Scan(&status, &failure)
		return err == nil && status == "error" && failure == "provider failed"
	})
	var count int
	if err := b.m.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM thread_state WHERE thread_id IN (?, ?)`, archived.ID, own.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cached archived or local state: %d, %v", count, err)
	}
	if err := b.m.SetEnabled(ctx, a.m.log.Node(), false); err != nil {
		t.Fatal(err)
	}
	got, err := b.store.Threads().Get(ctx, thread.ID)
	if err != nil || !got.StateStale || got.Status != "error" {
		t.Fatalf("offline state = %+v: %v", got, err)
	}
}
