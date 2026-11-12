package peer

import (
	"sync"
	"time"
)

// State is the presence of one peer.
type State string

const (
	Connected    State = "connected"
	Seen         State = "seen"
	Disconnected State = "disconnected"
)

// Intervals are the presence timings; zero fields take the defaults. Tests
// shrink them.
type Intervals struct {
	// Heartbeat is how often a subscription sends a heartbeat (default 15s).
	Heartbeat time.Duration
	// Timeout is how long without any heartbeat before a link counts as dead
	// (default 3 heartbeats).
	Timeout time.Duration
	// SeenWindow is how long after the last sign of life a peer shows as
	// "seen X ago" before turning disconnected (default 10 minutes).
	SeenWindow time.Duration
	// BackoffMin and BackoffMax bound the connector retry delay (1s, 30s).
	BackoffMin, BackoffMax time.Duration
	// Probe is the per-address dial timeout of the connector (default 3s).
	Probe time.Duration
}

func (v Intervals) heartbeat() time.Duration { return orDur(v.Heartbeat, 15*time.Second) }
func (v Intervals) timeout() time.Duration {
	return orDur(v.Timeout, 3*v.heartbeat())
}
func (v Intervals) seenWindow() time.Duration { return orDur(v.SeenWindow, 10*time.Minute) }
func (v Intervals) backoffMin() time.Duration { return orDur(v.BackoffMin, time.Second) }
func (v Intervals) backoffMax() time.Duration { return orDur(v.BackoffMax, 30*time.Second) }
func (v Intervals) probe() time.Duration      { return orDur(v.Probe, 3*time.Second) }

func orDur(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}

// Status is the presence of a peer at a moment.
type Status struct {
	DeviceID string        `json:"device_id"`
	State    State         `json:"state"`
	LastSeen time.Time     `json:"last_seen,omitempty"`
	Ago      time.Duration `json:"ago,omitempty"` // set when State is Seen
}

type presenceEntry struct {
	links int
	last  time.Time
}

// Presence tracks, in memory only, which peers are linked. It is never
// replicated: every node observes its own links.
type Presence struct {
	iv  Intervals
	now func() time.Time
	mu  sync.Mutex
	m   map[string]*presenceEntry
}

// NewPresence returns an empty tracker.
func NewPresence(iv Intervals) *Presence {
	return &Presence{iv: iv, now: time.Now, m: map[string]*presenceEntry{}}
}

func (p *Presence) entry(id string) *presenceEntry {
	e := p.m[id]
	if e == nil {
		e = &presenceEntry{}
		p.m[id] = e
	}
	return e
}

// Up records that a link (outgoing subscription or incoming stream) opened.
func (p *Presence) Up(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e := p.entry(id)
	e.links++
	e.last = p.now()
}

// Beat records a sign of life: a heartbeat or an operation batch.
func (p *Presence) Beat(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entry(id).last = p.now()
}

// Down records that a link closed.
func (p *Presence) Down(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e := p.entry(id)
	if e.links > 0 {
		e.links--
	}
}

// Status returns the presence of id.
func (p *Presence) Status(id string) Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status(id)
}

func (p *Presence) status(id string) Status {
	e := p.m[id]
	if e == nil || e.last.IsZero() {
		return Status{DeviceID: id, State: Disconnected}
	}
	ago := p.now().Sub(e.last)
	s := Status{DeviceID: id, LastSeen: e.last}
	switch {
	case e.links > 0 && ago <= p.iv.timeout():
		s.State = Connected
	case ago <= p.iv.seenWindow():
		s.State, s.Ago = Seen, ago
	default:
		s.State = Disconnected
	}
	return s
}

// All returns the status of every peer ever seen.
func (p *Presence) All() []Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Status, 0, len(p.m))
	for id := range p.m {
		out = append(out, p.status(id))
	}
	return out
}
