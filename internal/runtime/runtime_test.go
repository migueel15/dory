package runtime

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/nodestate"
	"github.com/migueel15/dory/internal/oplog"
	"github.com/migueel15/dory/internal/workspace"
)

// Event is what test providers emit.
type Event = harness.Event

// scripted is a test session over a channel of events.
type scripted struct {
	events  <-chan Event
	respond func(string, [][]string) error
}

func (s scripted) Events() <-chan Event { return s.events }
func (s scripted) Respond(id string, a [][]string) error {
	if s.respond == nil {
		return ErrNoRequest
	}
	return s.respond(id, a)
}

func adapt(events <-chan Event, respond func(string, [][]string) error) Session {
	return scripted{events, respond}
}

func manager(t *testing.T, delay time.Duration) (*Manager, *data.Store) {
	t.Helper()
	ctx := context.Background()
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
	store := data.New(db, log)
	file, _ := filepath.Abs(filepath.Join("testdata", "claude.jsonl"))
	return NewManager(db, store, log.Node(), dir, nodestate.New(db), nil, Replay{File: file, Delay: delay}), store
}

func wait(t *testing.T, store *data.Store, id, status string) data.Thread {
	t.Helper()
	for range 200 {
		th, err := store.Threads().Get(context.Background(), id)
		if err == nil && th.Status == status {
			return th
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("thread never reached %s", status)
	return data.Thread{}
}

func TestManagerRunsAndQueues(t *testing.T) {
	ctx := context.Background()
	m, store := manager(t, 2*time.Millisecond)
	updates, stop := m.Subscribe("")
	defer stop()
	th, err := registerTestThread(ctx, m, data.Thread{Provider: "replay", Title: "Prueba"})
	if err != nil {
		t.Fatal(err)
	}
	var before int
	if err := m.db.QueryRowContext(ctx, "SELECT count(*) FROM ops").Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err = m.Send(ctx, th.ID, "lista"); err != nil {
		t.Fatal(err)
	}
	if err = m.Send(ctx, th.ID, "otra vez"); err != nil {
		t.Fatal(err)
	}
	if q := m.Queue(th.ID); len(q) != 1 || q[0].Text != "otra vez" {
		t.Fatalf("queue: %+v", q)
	}
	for range 300 {
		if !m.Running(th.ID) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	// The queued turn starts a moment after the first one ends.
	users := 0
	for range 300 {
		th = wait(t, store, th.ID, "idle")
		msgs, err := m.Messages(ctx, th.ID)
		if err != nil {
			t.Fatal(err)
		}
		users = 0
		for _, msg := range msgs {
			if msg.Role == "user" {
				users++
			}
		}
		if users == 2 && !m.Running(th.ID) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Both turns ran; the replay reuses message ids, so each turn adds its user message.
	if users != 2 || !th.Unread || th.ReadAt != nil || th.LastActivityAt == nil {
		t.Fatalf("users=%d thread=%+v", users, th)
	}
	var after int
	if err := m.db.QueryRowContext(ctx, "SELECT count(*) FROM ops").Scan(&after); err != nil || after != before {
		t.Fatalf("turns changed log size from %d to %d: %v", before, after, err)
	}
	var session string
	m.db.QueryRow(`SELECT session_id FROM thread_runtime WHERE thread_id = ?`, th.ID).Scan(&session)
	if session != "s-1" {
		t.Fatalf("session not kept: %q", session)
	}
	if len(updates) == 0 || len(m.Usage()["replay"]) != 2 {
		t.Fatal("no updates or usage published")
	}
}

func TestManagerInterrupt(t *testing.T) {
	ctx := context.Background()
	m, store := manager(t, 50*time.Millisecond)
	th, _ := registerTestThread(ctx, m, data.Thread{Provider: "replay"})
	m.Send(ctx, th.ID, "larga")
	time.Sleep(20 * time.Millisecond)
	if err := m.Interrupt(ctx, th.ID); err != nil {
		t.Fatal(err)
	}
	wait(t, store, th.ID, "idle")
	msgs, _ := m.Messages(ctx, th.ID)
	if last := msgs[len(msgs)-1]; last.Role != "interrupt" || last.Code != "turn.interrupted" {
		t.Fatalf("last message %+v", last)
	}
}

func TestRecoverMarksDeadTurns(t *testing.T) {
	ctx := context.Background()
	m, store := manager(t, 0)
	th, _ := registerTestThread(ctx, m, data.Thread{Provider: "replay"})
	if err := store.SetThreadState(ctx, th.ID, "working", ""); err != nil {
		t.Fatal(err)
	}
	if err := m.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Threads().Get(ctx, th.ID); got.Status != "error" {
		t.Fatalf("status %s", got.Status)
	}
	msgs, _ := m.Messages(ctx, th.ID)
	if last := msgs[len(msgs)-1]; last.Role != "system" || last.Code != "node.restarted" || last.Text != "" {
		t.Fatalf("message %+v", last)
	}
}

func TestClearMessageHasCode(t *testing.T) {
	ctx := context.Background()
	m, _ := manager(t, 0)
	th, _ := registerTestThread(ctx, m, data.Thread{Provider: "replay"})
	if err := m.Clear(ctx, th.ID); err != nil {
		t.Fatal(err)
	}
	msgs, _ := m.Messages(ctx, th.ID)
	if last := msgs[len(msgs)-1]; last.Role != "clear" || last.Code != "context.cleared" {
		t.Fatalf("message %+v", last)
	}
}

func TestWorktreePerThread(t *testing.T) {
	ctx := context.Background()
	m, _ := manager(t, 0)
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.email=a@b", "-c", "user.name=a", "commit", "-q", "--allow-empty", "-m", "base"}} {
		if out, err := workspace.Run(ctx, repo, args...); err != nil {
			t.Fatalf("%s %v", out, err)
		}
	}
	th := data.Thread{ID: data.NewThreadID(), NodeID: m.node, Provider: "replay", Branch: "feat/1-prueba"}
	dir := filepath.Join(m.dataDir, "worktrees", th.ID)
	if err := workspace.AddWorktree(ctx, repo, dir, th.Branch, "main"); err != nil {
		t.Fatal(err)
	}
	th, err := m.Register(ctx, th, dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	dir, _ = m.Dir(ctx, th.ID)
	os.WriteFile(filepath.Join(dir, "nuevo.txt"), []byte("hola\n"), 0o644)
	workspace.Run(ctx, dir, "add", "nuevo.txt")
	diff, err := m.Diff(ctx, th.ID)
	if err != nil || !bytes.Contains([]byte(diff), []byte("+hola")) {
		t.Fatalf("diff %q %v", diff, err)
	}
	if out, _ := workspace.Run(ctx, dir, "branch", "--show-current"); string(bytes.TrimSpace(out)) != "feat/1-prueba" {
		t.Fatalf("branch %q", out)
	}
}

func TestAskWaitsForTheAnswer(t *testing.T) {
	m, _ := manager(t, 0)
	ctx := context.Background()
	th, err := registerTestThread(ctx, m, data.Thread{Provider: "replay", Title: "Pregunta"})
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan [][]string, 1)
	go func() {
		a, err := m.Ask(ctx, th.ID, []Question{{Question: "¿Seguimos?", Options: []Option{{Label: "Sí"}, {Label: "No"}}}})
		if err != nil {
			t.Error(err)
		}
		got <- a
	}()
	var req Request
	for range 200 {
		if rs, _ := m.Requests(ctx, th.ID); len(rs) == 1 {
			req = rs[0]
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if req.ID == "" {
		t.Fatal("ask_user left no pending request")
	}
	if err := m.Answer(ctx, th.ID, req.ID, [][]string{{"Sí"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case a := <-got:
		if len(a) != 1 || a[0][0] != "Sí" {
			t.Fatalf("answers = %v", a)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ask_user did not return the answer")
	}
	if rs, _ := m.Requests(ctx, th.ID); len(rs) != 0 {
		t.Fatalf("requests left: %v", rs)
	}
}

func testNodeState(t *testing.T) *nodestate.Store {
	t.Helper()
	db, err := localdb.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return nodestate.New(db)
}

func registerTestThread(ctx context.Context, m *Manager, thread data.Thread) (data.Thread, error) {
	thread.ID, thread.NodeID = data.NewThreadID(), m.node
	return m.Register(ctx, thread, "", "")
}
