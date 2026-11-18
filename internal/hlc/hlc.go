// Package hlc implements a hybrid logical clock. Timestamps order changes
// across nodes even when their wall clocks drift, and compare as strings.
package hlc

import (
	"fmt"
	"sync"
	"time"
)

// Timestamp is wall time in milliseconds, a counter for events within the
// same millisecond and the node that made it, which breaks ties.
type Timestamp struct {
	Wall    int64
	Counter uint32
	Node    string
}

// String encodes t with fixed widths so that string order is time order.
func (t Timestamp) String() string {
	return fmt.Sprintf("%015d-%010d-%s", t.Wall, t.Counter, t.Node)
}

// Parse decodes a timestamp written by String. An empty string is the zero value.
func Parse(s string) (Timestamp, error) {
	var t Timestamp
	if s == "" {
		return t, nil
	}
	if len(s) < 27 || s[15] != '-' || s[26] != '-' {
		return t, fmt.Errorf("hlc: malformed timestamp %q", s)
	}
	if _, err := fmt.Sscanf(s[:26], "%015d-%010d", &t.Wall, &t.Counter); err != nil {
		return t, fmt.Errorf("hlc: malformed timestamp %q", s)
	}
	t.Node = s[27:]
	return t, nil
}

// Less orders by wall time, then counter, then node.
func (t Timestamp) Less(o Timestamp) bool {
	if t.Wall != o.Wall {
		return t.Wall < o.Wall
	}
	if t.Counter != o.Counter {
		return t.Counter < o.Counter
	}
	return t.Node < o.Node
}

// Clock hands out increasing timestamps for one node.
type Clock struct {
	mu   sync.Mutex
	last Timestamp
	node string
	now  func() time.Time
}

// New starts a clock for node after last, the latest timestamp it issued or
// saw before a restart.
func New(node string, last Timestamp) *Clock {
	return &Clock{last: last, node: node, now: time.Now}
}

// Now returns a timestamp later than every one issued or observed.
func (c *Clock) Now() Timestamp {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.advance(c.now().UnixMilli(), Timestamp{})
}

// Observe merges a remote timestamp so later local ones sort after it.
func (c *Clock) Observe(remote Timestamp) Timestamp {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.advance(c.now().UnixMilli(), remote)
}

func (c *Clock) advance(wall int64, remote Timestamp) Timestamp {
	next := Timestamp{Wall: max(wall, c.last.Wall, remote.Wall), Node: c.node}
	switch {
	case next.Wall == c.last.Wall && next.Wall == remote.Wall:
		next.Counter = max(c.last.Counter, remote.Counter) + 1
	case next.Wall == c.last.Wall:
		next.Counter = c.last.Counter + 1
	case next.Wall == remote.Wall:
		next.Counter = remote.Counter + 1
	}
	c.last = next
	return next
}
