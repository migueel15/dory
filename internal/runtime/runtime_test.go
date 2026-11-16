package runtime

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/oplog"
)

func parseFile(t *testing.T, name string, parse func([]byte, func(Event))) []Event {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	for _, line := range bytes.Split(raw, []byte("\n")) {
		parse(line, func(e Event) { events = append(events, e) })
	}
	return events
}

// final keeps the last version of each message, as the store would.
func final(events []Event) (map[string]Message, []string) {
	msgs := map[string]Message{}
	var order []string
	for _, e := range events {
		if e.Message != nil {
			if _, ok := msgs[e.Message.ID]; !ok {
				order = append(order, e.Message.ID)
			}
			msgs[e.Message.ID] = *e.Message
		}
	}
	return msgs, order
}

func TestClaudeParser(t *testing.T) {
	p := &claudeParser{run: "r1", texts: map[string]*Message{}, tools: map[string]*Message{}}
	events := parseFile(t, "claude.jsonl", p.parse)
	if events[0].Session != "s-1" {
		t.Fatalf("session: %+v", events[0])
	}
	msgs, order := final(events)
	want := []struct{ id, role, text, status string }{
		{"msg_1:0", "reasoning", "Miro el repo", ""},
		{"msg_1:1", "assistant", "Voy a listar.", ""},
		{"tu_1", "tool", "go.mod\nREADME.md", "completed"},
		{"msg_2:0", "assistant", "Hecho.", ""},
	}
	if len(order) != len(want) {
		t.Fatalf("messages %v", order)
	}
	for i, w := range want {
		m := msgs[order[i]]
		if m.ID != w.id || m.Role != w.role || m.Text != w.text || m.Status != w.status || m.Streaming {
			t.Errorf("message %d: %+v", i, m)
		}
	}
	if msgs["tu_1"].Title != "Bash" || msgs["tu_1"].Detail != "ls -la" {
		t.Errorf("tool: %+v", msgs["tu_1"])
	}
	var usage []Window
	for _, e := range events {
		if e.Usage != nil {
			usage = e.Usage
		}
	}
	if len(usage) != 2 || usage[0].Kind != "five_hour" || usage[0].Utilization != 0.07 {
		t.Errorf("usage: %+v", usage)
	}
	if last := events[len(events)-1]; !last.Done || last.Err != "" {
		t.Errorf("end: %+v", last)
	}
}

func TestCodexParser(t *testing.T) {
	p := &codexParser{run: "r1"}
	events := parseFile(t, "codex.jsonl", p.parse)
	if events[0].Session != "c-1" {
		t.Fatalf("session: %+v", events[0])
	}
	msgs, order := final(events)
	if len(order) != 4 {
		t.Fatalf("messages %v (config warning must be skipped)", order)
	}
	if m := msgs["r1:item_2"]; m.Role != "tool" || m.Status != "completed" || m.Detail != "bash -lc ls" || m.Text != "go.mod" {
		t.Errorf("command: %+v", m)
	}
	if m := msgs["r1:item_3"]; m.Detail != "update a.go" {
		t.Errorf("file change: %+v", m)
	}
	if m := msgs["r1:item_4"]; m.Role != "assistant" || m.Text != "hola" {
		t.Errorf("answer: %+v", m)
	}
	if !events[len(events)-1].Done {
		t.Error("turn not finished")
	}
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
	return NewManager(db, store, log.Node(), dir, Replay{File: file, Delay: delay}), store
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
	th, err := m.Create(ctx, NewThread{Provider: "replay", Title: "Prueba"})
	if err != nil {
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
	th = wait(t, store, th.ID, "idle")
	msgs, err := m.Messages(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	users := 0
	for _, msg := range msgs {
		if msg.Role == "user" {
			users++
		}
	}
	// Both turns ran; the replay reuses message ids, so each turn adds its user message.
	if users != 2 || th.ReadAt != nil || th.LastActivityAt == nil {
		t.Fatalf("users=%d thread=%+v", users, th)
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
	th, _ := m.Create(ctx, NewThread{Provider: "replay"})
	m.Send(ctx, th.ID, "larga")
	time.Sleep(20 * time.Millisecond)
	if err := m.Interrupt(ctx, th.ID); err != nil {
		t.Fatal(err)
	}
	wait(t, store, th.ID, "idle")
	msgs, _ := m.Messages(ctx, th.ID)
	if last := msgs[len(msgs)-1]; last.Role != "interrupt" {
		t.Fatalf("last message %+v", last)
	}
}

func TestRecoverMarksDeadTurns(t *testing.T) {
	ctx := context.Background()
	m, store := manager(t, 0)
	th, _ := m.Create(ctx, NewThread{Provider: "replay"})
	th.Status = "working"
	store.Threads().Put(ctx, th)
	if err := m.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Threads().Get(ctx, th.ID); got.Status != "error" {
		t.Fatalf("status %s", got.Status)
	}
}

func TestWorktreePerThread(t *testing.T) {
	ctx := context.Background()
	m, _ := manager(t, 0)
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.email=a@b", "-c", "user.name=a", "commit", "-q", "--allow-empty", "-m", "base"}} {
		if out, err := gitCmd(ctx, repo, args...); err != nil {
			t.Fatalf("%s %v", out, err)
		}
	}
	th, err := m.Create(ctx, NewThread{Provider: "replay", Worktree: true, RepoPath: repo, Branch: "feat/1-prueba", BaseBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := m.Dir(ctx, th.ID)
	os.WriteFile(filepath.Join(dir, "nuevo.txt"), []byte("hola\n"), 0o644)
	gitCmd(ctx, dir, "add", "nuevo.txt")
	diff, err := m.Diff(ctx, th.ID)
	if err != nil || !bytes.Contains([]byte(diff), []byte("+hola")) {
		t.Fatalf("diff %q %v", diff, err)
	}
	if out, _ := gitCmd(ctx, dir, "branch", "--show-current"); string(bytes.TrimSpace(out)) != "feat/1-prueba" {
		t.Fatalf("branch %q", out)
	}
}
