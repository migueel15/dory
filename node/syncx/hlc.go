// Package syncx holds what replication needs below the wire protocol: the
// hybrid logical clock, the operation log and the deterministic merge.
package syncx

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HLC is a hybrid logical clock timestamp. Wall is Unix milliseconds, Counter
// breaks ties inside one millisecond and Device breaks ties between devices, so
// any two stamps are totally ordered.
type HLC struct {
	Wall    int64
	Counter int
	Device  string
}

const maxCounter = 0xffff

// String is fixed width up to the '@', so comparing strings compares stamps.
func (h HLC) String() string {
	return fmt.Sprintf("%012x-%04x@%s", h.Wall, h.Counter, h.Device)
}

// IsZero reports whether h is the zero stamp.
func (h HLC) IsZero() bool { return h.Wall == 0 && h.Counter == 0 && h.Device == "" }

// ParseHLC parses the output of String.
func ParseHLC(s string) (HLC, error) {
	at := strings.IndexByte(s, '@')
	if at != 17 || s[12] != '-' {
		return HLC{}, fmt.Errorf("syncx: invalid hlc %q", s)
	}
	wall, err := strconv.ParseInt(s[:12], 16, 64)
	if err != nil {
		return HLC{}, fmt.Errorf("syncx: invalid hlc %q", s)
	}
	counter, err := strconv.ParseInt(s[13:17], 16, 32)
	if err != nil {
		return HLC{}, fmt.Errorf("syncx: invalid hlc %q", s)
	}
	return HLC{Wall: wall, Counter: int(counter), Device: s[18:]}, nil
}

// Compare orders two stamps: -1, 0 or 1.
func (h HLC) Compare(o HLC) int { return strings.Compare(h.String(), o.String()) }

// Time is the wall component as a time.
func (h HLC) Time() time.Time { return time.UnixMilli(h.Wall).UTC() }

// Clock issues stamps for one device. Every stamp it returns is greater than
// any stamp it issued or observed before.
type Clock struct {
	mu     sync.Mutex
	device string
	now    func() time.Time
	last   HLC
}

// NewClock returns a clock for device. now defaults to time.Now.
func NewClock(device string, now func() time.Time) *Clock {
	if now == nil {
		now = time.Now
	}
	return &Clock{device: device, now: now}
}

// Seed makes the clock continue after h (used when reopening a database).
func (c *Clock) Seed(h HLC) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if h.Wall > c.last.Wall || (h.Wall == c.last.Wall && h.Counter > c.last.Counter) {
		c.last = HLC{Wall: h.Wall, Counter: h.Counter}
	}
}

func (c *Clock) advance(wall int64, counter int) HLC {
	if counter > maxCounter {
		wall, counter = wall+1, 0
	}
	c.last = HLC{Wall: wall, Counter: counter}
	return HLC{Wall: wall, Counter: counter, Device: c.device}
}

// Now issues a stamp for a local event.
func (c *Clock) Now() HLC {
	c.mu.Lock()
	defer c.mu.Unlock()
	phys := c.now().UnixMilli()
	if phys > c.last.Wall {
		return c.advance(phys, 0)
	}
	return c.advance(c.last.Wall, c.last.Counter+1)
}

// Observe folds a stamp received from another device into the clock and issues
// the stamp for the receive event.
func (c *Clock) Observe(r HLC) HLC {
	c.mu.Lock()
	defer c.mu.Unlock()
	phys := c.now().UnixMilli()
	wall := max(phys, c.last.Wall, r.Wall)
	counter := 0
	switch {
	case wall == c.last.Wall && wall == r.Wall:
		counter = max(c.last.Counter, r.Counter) + 1
	case wall == c.last.Wall:
		counter = c.last.Counter + 1
	case wall == r.Wall:
		counter = r.Counter + 1
	}
	return c.advance(wall, counter)
}
