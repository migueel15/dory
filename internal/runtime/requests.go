package runtime

import (
	"context"
	"encoding/json"
	"log"
)

// request records a request of the agent; the thread waits for the user.
func (m *Manager) request(ctx context.Context, thread string, r Request) {
	if r.CreatedAt == "" {
		r.CreatedAt = now()
	}
	qs, _ := json.Marshal(r.Questions)
	if _, err := m.db.ExecContext(ctx, `INSERT INTO thread_requests(id, thread_id, kind, message_id, questions, created_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING`, r.ID, thread, r.Kind, r.MessageID, string(qs), r.CreatedAt); err != nil {
		log.Printf("runtime: save request: %v", err)
		return
	}
	m.setStatus(ctx, thread, "waiting", "")
	m.publish(Update{ThreadID: thread, Reload: true})
}

// Requests returns what the agent of a thread is waiting for, oldest first.
func (m *Manager) Requests(ctx context.Context, thread string) ([]Request, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT id, kind, message_id, questions, created_at FROM thread_requests WHERE thread_id = ? ORDER BY created_at`, thread)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Request{}
	for rows.Next() {
		r := Request{ThreadID: thread}
		var qs string
		if err = rows.Scan(&r.ID, &r.Kind, &r.MessageID, &qs, &r.CreatedAt); err != nil {
			return nil, err
		}
		if r.Kind == "secret" {
			r.Secret = &SecretPrompt{}
			_ = json.Unmarshal([]byte(qs), r.Secret)
		} else {
			_ = json.Unmarshal([]byte(qs), &r.Questions)
		}
		if r.Questions == nil {
			r.Questions = []Question{}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Answer resolves a pending request; the turn goes on with the answers.
func (m *Manager) Answer(ctx context.Context, thread, id string, answers [][]string) error {
	if _, err := m.local(ctx, thread); err != nil {
		return err
	}
	m.mu.Lock()
	s := m.sessions[thread]
	waiter, asked := m.waiters[id]
	delete(m.waiters, id)
	m.mu.Unlock()
	if asked {
		// An ask_user call: its tool call returns the answers.
		waiter <- answers
		return m.resolved(ctx, thread, id)
	}
	var exists bool
	if err := m.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM thread_requests WHERE id = ? AND thread_id = ? AND kind <> 'secret')`, id, thread).Scan(&exists); err != nil {
		return err
	}
	if s == nil || !exists {
		return ErrNoRequest
	}
	if err := s.Answer(id, answers); err != nil {
		return err
	}
	return m.resolved(ctx, thread, id)
}

// resolved forgets an answered request; the turn goes on.
func (m *Manager) resolved(ctx context.Context, thread, id string) error {
	if _, err := m.db.ExecContext(ctx, `DELETE FROM thread_requests WHERE id = ?`, id); err != nil {
		return err
	}
	var left bool
	m.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM thread_requests WHERE thread_id = ?)`, thread).Scan(&left)
	if !left {
		m.setStatus(ctx, thread, "working", "")
	}
	m.publish(Update{ThreadID: thread, Reload: true})
	return nil
}

// dropRequests forgets the requests whose process died with the node; their
// threads end in error.
func (m *Manager) dropRequests(ctx context.Context) error {
	rows, err := m.db.QueryContext(ctx, `SELECT DISTINCT thread_id FROM thread_requests`)
	if err != nil {
		return err
	}
	var threads []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			threads = append(threads, id)
		}
	}
	rows.Close()
	for _, id := range threads {
		if t, err := m.local(ctx, id); err != nil || t.Status != "waiting" {
			continue
		}
		m.setStatus(ctx, id, "error", "")
		m.save(ctx, Message{ID: newRunID(), ThreadID: id, Role: "system", Text: "El nodo se reinició mientras el agente esperaba una respuesta; el turno se interrumpió.", CreatedAt: now()})
	}
	_, err = m.db.ExecContext(ctx, `DELETE FROM thread_requests`)
	return err
}
