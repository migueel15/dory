package syncx

import (
	"context"
	"errors"
	"sync"
)

// ErrDisconnected is returned by SyncWith when the link to the peer dropped.
// The caller reconnects by calling SyncWith again: the version vectors make
// the resume exact, with no duplicates and no losses.
var ErrDisconnected = errors.New("syncx: peer disconnected")

// DefaultBatch is the number of operations pulled and applied per transaction.
const DefaultBatch = 500

// Transport is how a node talks to one peer. HTTP implements it in a later
// step; MemTransport implements it for tests.
type Transport interface {
	// Vector is the peer's version vector.
	Vector(ctx context.Context) (VersionVector, error)
	// Pull returns up to limit operations the holder of since lacks, ordered
	// by stamp.
	Pull(ctx context.Context, since VersionVector, limit int) ([]Op, error)
	// Subscribe streams batches of operations the holder of since lacks, first
	// the backlog and then new ones as they appear. The channel closes when
	// ctx ends or the link drops.
	Subscribe(ctx context.Context, since VersionVector) (<-chan []Op, error)
}

// Snapshotter is implemented by transports that can serve the full state in
// one consistent read, which is how an empty node bootstraps.
type Snapshotter interface {
	Snapshot(ctx context.Context) (VersionVector, []Op, error)
}

// Engine replicates the log of one node from its peers.
type Engine struct {
	Log *Log
	// Batch is the operations per transaction; zero means DefaultBatch.
	Batch int
	// OnChange is called after remote operations were applied, with only the
	// new ones. The SPA's SSE needs no hook of its own: replicated audit
	// events land in the events table that the stream polls, so a remote
	// change reaches `event: change` by itself. OnChange lets a host wake it
	// sooner.
	OnChange func(applied []Op)
}

func (e *Engine) batch() int {
	if e.Batch > 0 {
		return e.Batch
	}
	return DefaultBatch
}

func (e *Engine) apply(ctx context.Context, ops []Op) error {
	if len(ops) == 0 {
		return nil
	}
	applied, err := e.Log.ApplyNew(ctx, ops)
	if err != nil {
		return err
	}
	if len(applied) > 0 && e.OnChange != nil {
		e.OnChange(applied)
	}
	return nil
}

// SyncWith pulls from peer what this node lacks, in stamp-ordered batches
// each applied in one transaction, then keeps a subscription open and applies
// new operations as they arrive. Operations that originated on third devices
// are included. It blocks until ctx ends (returning ctx.Err()) or the link
// drops (ErrDisconnected). Replication is one way: call it on both nodes for a
// two way link.
func (e *Engine) SyncWith(ctx context.Context, peer Transport) error {
	local, err := e.Log.Vector(ctx)
	if err != nil {
		return err
	}
	if len(local) == 0 {
		if s, ok := peer.(Snapshotter); ok {
			if err = e.bootstrap(ctx, s); err != nil {
				return err
			}
		}
	}
	for {
		if local, err = e.Log.Vector(ctx); err != nil {
			return err
		}
		ops, err := peer.Pull(ctx, local, e.batch())
		if err != nil {
			return err
		}
		if len(ops) == 0 {
			break
		}
		if err = e.apply(ctx, ops); err != nil {
			return err
		}
	}
	local, err = e.Log.Vector(ctx)
	if err != nil {
		return err
	}
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, err := peer.Subscribe(subCtx, local)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ops, ok := <-ch:
			if !ok {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return ErrDisconnected
			}
			for len(ops) > 0 {
				n := min(len(ops), e.batch())
				if err = e.apply(ctx, ops[:n]); err != nil {
					return err
				}
				ops = ops[n:]
			}
		}
	}
}

// bootstrap loads the peer's full state into an empty node, a batch per
// transaction. The snapshot's vector tells where the op stream continues.
func (e *Engine) bootstrap(ctx context.Context, s Snapshotter) error {
	_, ops, err := s.Snapshot(ctx)
	if err != nil {
		return err
	}
	for len(ops) > 0 {
		n := min(len(ops), e.batch())
		if err = e.apply(ctx, ops[:n]); err != nil {
			return err
		}
		ops = ops[n:]
	}
	return nil
}

// MemTransport serves the log of a node in memory, with a switch to cut and
// restore the link.
type MemTransport struct {
	remote *Log
	mu     sync.Mutex
	cut    bool
	cutCh  chan struct{}
}

// NewMemTransport returns a link to the node that owns remote.
func NewMemTransport(remote *Log) *MemTransport {
	return &MemTransport{remote: remote, cutCh: make(chan struct{})}
}

// Cut drops the link: calls fail and open subscriptions close.
func (m *MemTransport) Cut() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.cut {
		m.cut = true
		close(m.cutCh)
	}
}

// Restore reopens the link after Cut.
func (m *MemTransport) Restore() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cut {
		m.cut = false
		m.cutCh = make(chan struct{})
	}
}

func (m *MemTransport) state() (chan struct{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cut {
		return nil, ErrDisconnected
	}
	return m.cutCh, nil
}

func (m *MemTransport) Vector(ctx context.Context) (VersionVector, error) {
	if _, err := m.state(); err != nil {
		return nil, err
	}
	return m.remote.Vector(ctx)
}

func (m *MemTransport) Pull(ctx context.Context, since VersionVector, limit int) ([]Op, error) {
	if _, err := m.state(); err != nil {
		return nil, err
	}
	return m.remote.Since(ctx, since, limit)
}

func (m *MemTransport) Snapshot(ctx context.Context) (VersionVector, []Op, error) {
	if _, err := m.state(); err != nil {
		return nil, nil, err
	}
	return m.remote.Snapshot(ctx)
}

func (m *MemTransport) Subscribe(ctx context.Context, since VersionVector) (<-chan []Op, error) {
	cut, err := m.state()
	if err != nil {
		return nil, err
	}
	pos := VersionVector{}
	for d, s := range since {
		pos[d] = s
	}
	wake, cancel := m.remote.Watch()
	out := make(chan []Op)
	go func() {
		defer cancel()
		defer close(out)
		for {
			// Drain what is pending before waiting, so a signal that arrived
			// during a send is never lost.
			for {
				ops, err := m.remote.Since(ctx, pos, DefaultBatch)
				if err != nil || len(ops) == 0 {
					break
				}
				// Advance only over gapless seqs: ops after a gap are
				// re-sent once the gap fills, and the receiver ignores
				// duplicates.
				moved := false
				for _, o := range ops {
					if o.Seq == pos[o.DeviceID]+1 {
						pos[o.DeviceID] = o.Seq
						moved = true
					}
				}
				select {
				case out <- ops:
				case <-ctx.Done():
					return
				case <-cut:
					return
				}
				if !moved {
					break
				}
			}
			select {
			case <-wake:
			case <-ctx.Done():
				return
			case <-cut:
				return
			}
		}
	}()
	return out, nil
}
