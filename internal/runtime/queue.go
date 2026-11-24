package runtime

import "context"

// EditQueued changes the text of a message waiting in a thread.
func (m *Manager) EditQueued(ctx context.Context, id, run, text string) error {
	if _, err := m.local(ctx, id); err != nil {
		return err
	}
	return m.do(id, &command{kind: cmdEditQueued, ctx: ctx, run: run, text: text}).err
}

// CancelQueued drops a waiting message.
func (m *Manager) CancelQueued(ctx context.Context, id, run string) error {
	if _, err := m.local(ctx, id); err != nil {
		return err
	}
	return m.do(id, &command{kind: cmdCancelQueued, ctx: ctx, run: run}).err
}

// SteerQueued sends a waiting message now: the harnesses cannot take input in
// the middle of a turn, so the running turn stops and this message runs next.
func (m *Manager) SteerQueued(ctx context.Context, id, run string) error {
	if _, err := m.local(ctx, id); err != nil {
		return err
	}
	return m.do(id, &command{kind: cmdSteer, ctx: ctx, run: run}).err
}
