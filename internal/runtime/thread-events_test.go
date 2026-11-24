package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/migueel15/dory/internal/data"
)

func eventThread(t *testing.T) (*Manager, string) {
	t.Helper()
	m, store := manager(t, 0)
	th, err := store.Threads().Create(context.Background(), data.Thread{NodeID: m.node, Provider: "claude", Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	return m, th.ID
}

func TestEventSeqIsMonotonicWithoutGapsUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	m, id := eventThread(t)
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				m.save(ctx, Message{ID: fmt.Sprintf("m%d", i), ThreadID: id, Role: "assistant", Text: "x"})
			} else {
				m.emitJSON(ctx, id, EventQueueChanged, map[string]int{"n": i})
			}
		}()
	}
	wg.Wait()
	events, err := m.EventsAfter(ctx, id, 0, 100)
	if err != nil || len(events) != 40 {
		t.Fatalf("events = %d, err = %v", len(events), err)
	}
	for i, e := range events {
		if e.Seq != i+1 {
			t.Fatalf("event %d has seq %d", i, e.Seq)
		}
	}
}

func TestEventsAfterReturnsOnlyWhatIsMissing(t *testing.T) {
	ctx := context.Background()
	m, id := eventThread(t)
	for i := range 5 {
		m.save(ctx, Message{ID: fmt.Sprintf("m%d", i), ThreadID: id, Role: "user", Text: "x"})
	}
	events, _ := m.EventsAfter(ctx, id, 3, 100)
	if len(events) != 2 || events[0].Seq != 4 || events[1].Seq != 5 || events[0].Type != EventItemCompleted {
		t.Fatalf("events = %+v", events)
	}
	var msg Message
	if json.Unmarshal(events[0].Payload, &msg) != nil || msg.ID != "m3" {
		t.Fatalf("payload = %s", events[0].Payload)
	}
}

func TestStreamingTextRecordsOnlyItsStartAndEnd(t *testing.T) {
	ctx := context.Background()
	m, id := eventThread(t)
	msg := Message{ID: "a", ThreadID: id, Role: "assistant", Text: "ho", Streaming: true}
	m.persist(ctx, &msg, EventItemStarted, "")
	msg.Text = "hola"
	m.persist(ctx, &msg, "", "") // a checkpoint: projection only
	msg.Streaming = false
	m.save(ctx, msg)
	events, _ := m.EventsAfter(ctx, id, 0, 10)
	if len(events) != 2 || events[0].Type != EventItemStarted || events[1].Type != EventItemCompleted {
		t.Fatalf("events = %+v", events)
	}
}

func TestSlowSubscriberIsClosedAndCanResume(t *testing.T) {
	ctx := context.Background()
	m, id := eventThread(t)
	live, slow, stop := m.SubscribeEvents(id)
	defer stop()
	for i := range 600 {
		m.emitJSON(ctx, id, EventQueueChanged, map[string]int{"n": i})
	}
	got := 0
	for range live {
		got++
	}
	if !slow() || got != 512 {
		t.Fatalf("slow = %v, got = %d", slow(), got)
	}
	// Nothing durable is lost: the record holds all of it.
	rest, _ := m.EventsAfter(ctx, id, got, 1000)
	if len(rest) != 600-got || rest[0].Seq != got+1 {
		t.Fatalf("rest = %d", len(rest))
	}
}

func TestClearIsRecordedAsMarker(t *testing.T) {
	ctx := context.Background()
	m, id := eventThread(t)
	m.save(ctx, Message{ID: "c", ThreadID: id, Role: "clear", Text: "Contexto limpiado"})
	events, _ := m.EventsAfter(ctx, id, 0, 10)
	if len(events) != 1 || events[0].Type != EventMarker {
		t.Fatalf("events = %+v", events)
	}
}

func TestMigrationBackfillsEventsForExistingMessages(t *testing.T) {
	// Messages written by older versions have no events; the migration made
	// one per message, so the lookup below works on a fresh database too.
	ctx := context.Background()
	m, id := eventThread(t)
	m.db.ExecContext(ctx, `INSERT INTO thread_messages(id, thread_id, role, text, created_at, updated_at) VALUES ('old', ?, 'user', 'hi', 'x', 'x')`, id)
	// Same statement as the migration's backfill, run on a message it did not see.
	if _, err := m.db.ExecContext(ctx, `INSERT INTO thread_events(thread_id, seq, run_id, type, item_id, payload, created_at)
		SELECT thread_id, ROW_NUMBER() OVER (PARTITION BY thread_id ORDER BY seq), run_id, 'item.completed', id,
		json_object('id', id, 'role', role, 'text', text), created_at FROM thread_messages WHERE thread_id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if m.EventSeq(ctx, id) != 1 {
		t.Fatal("no event for the existing message")
	}
}
