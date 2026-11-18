package runtime

import "context"

// EditQueued changes the text of a message waiting in a thread.
func (m *Manager) EditQueued(ctx context.Context, id, run, text string) error {
	return m.changeQueue(ctx, id, run, func(q []Queued, i int) []Queued {
		q[i].Text = text
		return q
	})
}

// CancelQueued drops a waiting message.
func (m *Manager) CancelQueued(ctx context.Context, id, run string) error {
	return m.changeQueue(ctx, id, run, func(q []Queued, i int) []Queued {
		return append(q[:i], q[i+1:]...)
	})
}

// SteerQueued sends a waiting message now: the harnesses cannot take input in
// the middle of a turn, so the running turn stops and this message runs next.
func (m *Manager) SteerQueued(ctx context.Context, id, run string) error {
	err := m.changeQueue(ctx, id, run, func(q []Queued, i int) []Queued {
		head := q[i]
		return append([]Queued{head}, append(q[:i:i], q[i+1:]...)...)
	})
	if err != nil {
		return err
	}
	m.mu.Lock()
	cancel := m.running[id]
	if cancel != nil {
		m.steering[id] = true
	}
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (m *Manager) changeQueue(ctx context.Context, id, run string, change func([]Queued, int) []Queued) error {
	if _, err := m.local(ctx, id); err != nil {
		return err
	}
	m.mu.Lock()
	q := m.queues[id]
	i := -1
	for j, x := range q {
		if x.RunID == run {
			i = j
		}
	}
	if i < 0 {
		m.mu.Unlock()
		return ErrNotFound
	}
	m.queues[id] = change(append([]Queued(nil), q...), i)
	out := append([]Queued{}, m.queues[id]...)
	m.mu.Unlock()
	m.publish(Update{ThreadID: id, Queue: out})
	return nil
}
