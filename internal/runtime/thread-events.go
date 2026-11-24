package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
)

// ThreadEvent is one entry of the record of a thread. Durable events carry a
// Seq, numbered per thread without gaps; deltas (the fragments of a text that
// is being written) have none and only travel live.
type ThreadEvent struct {
	ThreadID  string          `json:"thread_id"`
	Seq       int             `json:"seq,omitempty"`
	RunID     string          `json:"run_id,omitempty"`
	Type      string          `json:"type"`
	ItemID    string          `json:"item_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	CreatedAt string          `json:"created_at,omitempty"`
}

// Durable event types.
const (
	EventTurnStarted     = "turn.started"
	EventItemStarted     = "item.started"
	EventItemCompleted   = "item.completed"
	EventRequestOpened   = "request.opened"
	EventRequestResolved = "request.resolved"
	EventQueueChanged    = "queue.changed"
	EventStateChanged    = "state.changed"
	EventSettings        = "settings.reported"
	EventSessionBound    = "session.bound"
	EventTurnEnded       = "turn.ended"
	EventMarker          = "marker"
	// EventDelta is live only.
	EventDelta = "delta"
)

// markerRoles are the messages that mark a point of the conversation.
var markerRoles = map[string]bool{"clear": true, "interrupt": true, "model-change": true, "changes": true}

// messageEventType says which event records a message: a marker, an item that
// is still going on, or a finished one.
func messageEventType(msg Message) string {
	switch {
	case markerRoles[msg.Role]:
		return EventMarker
	case msg.Status == "running" || msg.Status == "waiting":
		return EventItemStarted
	}
	return EventItemCompleted
}

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// appendEvent adds an event to the record of a thread with the next number.
// The actor is the only writer of its thread; the primary key backs that up.
func appendEvent(ctx context.Context, q execer, e ThreadEvent) (ThreadEvent, error) {
	if e.CreatedAt == "" {
		e.CreatedAt = now()
	}
	if len(e.Payload) == 0 {
		e.Payload = json.RawMessage(`{}`)
	}
	err := q.QueryRowContext(ctx, `INSERT INTO thread_events(thread_id, seq, run_id, type, item_id, payload, created_at)
		VALUES (?, (SELECT COALESCE(MAX(seq), 0) + 1 FROM thread_events WHERE thread_id = ?), ?, ?, ?, ?, ?) RETURNING seq`,
		e.ThreadID, e.ThreadID, e.RunID, e.Type, e.ItemID, string(e.Payload), e.CreatedAt).Scan(&e.Seq)
	return e, err
}

// emit records an event and sends it to the live subscribers. Recording and
// sending go together under one lock, so the live order is the numbering.
func (m *Manager) emit(ctx context.Context, e ThreadEvent) (ThreadEvent, bool) {
	m.evMu.Lock()
	defer m.evMu.Unlock()
	e, err := appendEvent(ctx, m.db, e)
	if err != nil {
		log.Printf("runtime: record event %s of %s: %v", e.Type, e.ThreadID, err)
		return e, false
	}
	m.sendEvent(e)
	return e, true
}

// emitJSON is emit with a payload to encode.
func (m *Manager) emitJSON(ctx context.Context, thread, typ string, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	m.emit(ctx, ThreadEvent{ThreadID: thread, Type: typ, Payload: raw})
}

// live sends an event that is not recorded: the fragments of a text.
func (m *Manager) live(e ThreadEvent) {
	m.evMu.Lock()
	defer m.evMu.Unlock()
	m.sendEvent(e)
}

// eventSub is a live subscription. When it cannot keep up its channel is
// closed instead of dropping events; the client resumes from the record.
type eventSub struct {
	ch     chan ThreadEvent
	thread string
	slow   bool
}

func (m *Manager) sendEvent(e ThreadEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for s := range m.evSubs {
		if s.thread != e.ThreadID {
			continue
		}
		select {
		case s.ch <- e:
		default:
			s.slow = true
			close(s.ch)
			delete(m.evSubs, s)
		}
	}
}

// SubscribeEvents receives the live events of a thread. The channel is closed
// when the subscriber falls behind or stops; slow tells which.
func (m *Manager) SubscribeEvents(thread string) (events <-chan ThreadEvent, slow func() bool, stop func()) {
	s := &eventSub{ch: make(chan ThreadEvent, 512), thread: thread}
	m.mu.Lock()
	m.evSubs[s] = struct{}{}
	m.mu.Unlock()
	return s.ch, func() bool {
			m.mu.Lock()
			defer m.mu.Unlock()
			return s.slow
		}, func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			if _, ok := m.evSubs[s]; ok {
				delete(m.evSubs, s)
				close(s.ch)
			}
		}
}

// EventsAfter returns the recorded events of a thread after seq, oldest first.
func (m *Manager) EventsAfter(ctx context.Context, thread string, after, limit int) ([]ThreadEvent, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT seq, run_id, type, item_id, payload, created_at FROM thread_events
		WHERE thread_id = ? AND seq > ? ORDER BY seq LIMIT ?`, thread, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ThreadEvent
	for rows.Next() {
		e := ThreadEvent{ThreadID: thread}
		var payload string
		if err := rows.Scan(&e.Seq, &e.RunID, &e.Type, &e.ItemID, &payload, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Payload = json.RawMessage(payload)
		out = append(out, e)
	}
	return out, rows.Err()
}

// EventSeq is the number of the last event of a thread; 0 when it has none.
func (m *Manager) EventSeq(ctx context.Context, thread string) int {
	var seq int
	m.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) FROM thread_events WHERE thread_id = ?`, thread).Scan(&seq)
	return seq
}

// deltaPayload is what a live fragment carries: enough to start the item on
// a client that has not seen it, and the new text.
func deltaPayload(msg Message, delta string, replace bool) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{"id": msg.ID, "role": msg.Role, "run_id": msg.RunID, "created_at": msg.CreatedAt, "text": delta, "replace": replace})
	return raw
}

// Save stores a message and records its event; for tools outside the runtime.
func (m *Manager) Save(ctx context.Context, msg Message) { m.save(ctx, msg) }
