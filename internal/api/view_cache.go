package api

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/t3"
	"golang.org/x/sync/singleflight"
)

// threadRead is what Dory reads from T3 to show a thread.
type threadRead struct {
	view   t3.ThreadView
	cursor string
}

// viewCache keeps the last read of each thread so opening one again, from
// another tab or device or after a hover prefetch, does not wait for T3.
// An entry is used only while nothing says the thread changed: no stream
// burst or write from Dory since it was read, the monitor reports the same
// update time, and the thread is not running.
type viewCache struct {
	mu      sync.Mutex
	entries map[string]viewEntry
	gen     map[string]uint64
	flight  singleflight.Group
}

type viewEntry struct {
	read    threadRead
	at      time.Time
	updated string
	gen     uint64
}

// Bounds of the cache: entries of threads the monitor does not list (settled
// long ago) are trusted for a short while only.
const (
	viewTTL         = 10 * time.Minute
	viewUnlistedTTL = 30 * time.Second
	viewMaxEntries  = 300
)

var views = &viewCache{entries: map[string]viewEntry{}, gen: map[string]uint64{}}

// forget drops a thread's entry and any read already in flight for it.
func (c *viewCache) forget(id string) {
	c.mu.Lock()
	c.gen[id]++
	delete(c.entries, id)
	c.mu.Unlock()
}

func busy(status string) bool {
	switch strings.ToLower(status) {
	case "running", "working", "starting", "preparing":
		return true
	}
	return false
}

// get returns the thread's read, from the cache when it is still valid.
func (c *viewCache) get(ctx context.Context, monitor *t3.Monitor, id string, fetch func(context.Context) (threadRead, error)) (threadRead, error) {
	live, listed := monitor.Thread(id)
	c.mu.Lock()
	gen := c.gen[id]
	e, ok := c.entries[id]
	c.mu.Unlock()
	if ok && e.gen == gen {
		age := time.Since(e.at)
		if listed && !busy(live.Status) && live.UpdatedAt == e.updated && age < viewTTL || !listed && age < viewUnlistedTTL {
			return e.read, nil
		}
	}
	// Concurrent opens of one thread share a single read; it outlives a caller
	// that gives up, so the others still get it.
	v, err, _ := c.flight.Do(id+":"+strconv.FormatUint(gen, 10), func() (any, error) {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		read, err := fetch(rctx)
		if err != nil {
			return threadRead{}, err
		}
		c.store(id, gen, live.UpdatedAt, read)
		return read, nil
	})
	if err != nil {
		return threadRead{}, err
	}
	return v.(threadRead), nil
}

func (c *viewCache) store(id string, gen uint64, updated string, read threadRead) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen[id] != gen {
		return // the thread changed while it was read
	}
	if len(c.entries) >= viewMaxEntries {
		for k, e := range c.entries {
			if time.Since(e.at) > viewTTL || len(c.entries) >= viewMaxEntries {
				delete(c.entries, k)
			}
		}
	}
	c.entries[id] = viewEntry{read: read, at: time.Now(), updated: updated, gen: gen}
}
