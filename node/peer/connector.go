package peer

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/migueel15/dory/node/localstore"
	"github.com/migueel15/dory/node/syncx"
)

// AddressBook gives the replicated address record of a device.
type AddressBook interface {
	AddressOf(ctx context.Context, deviceID string) (localstore.DeviceAddress, bool, error)
}

// Candidate is one address to try and where it came from.
type Candidate struct {
	Source string // last, mdns, lan, tailnet, manual
	Addr   string
}

// Connector keeps a sync session open with every other group member. For each
// one it probes in parallel the last working address, mDNS, the LAN addresses
// and tailnet name from the replicated record, and manual addresses; the first
// that answers wins. Failures retry with exponential backoff.
type Connector struct {
	Identity *Identity
	Group    *Log
	Engine   *syncx.Engine
	Presence *Presence
	Book     AddressBook // optional
	MDNS     *MDNS       // optional
	// Manual are extra addresses by device id (explicit IP:port injection).
	Manual    map[string][]string
	Intervals Intervals
	// ScanEvery is how often the membership is re-read (default 5s).
	ScanEvery time.Duration
	// OnConnect is called when a link to a device is established.
	OnConnect func(deviceID string, c Candidate)

	mu      sync.Mutex
	last    map[string]string
	running map[string]bool
	wake    chan struct{} // closed (and replaced) by Kick
}

// Kick wakes every peer loop that is waiting to retry, so the node reconnects
// right away (the app came back to the foreground, the network changed).
// Links that are already syncing are untouched.
func (c *Connector) Kick() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.wake != nil {
		close(c.wake)
	}
	c.wake = make(chan struct{})
}

func (c *Connector) kicked() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.wake == nil {
		c.wake = make(chan struct{})
	}
	return c.wake
}

// Candidates returns the addresses to try for deviceID.
func (c *Connector) Candidates(ctx context.Context, deviceID string) []Candidate {
	var out []Candidate
	seen := map[string]bool{}
	add := func(src, addr string) {
		if addr != "" && !seen[addr] {
			seen[addr] = true
			out = append(out, Candidate{src, addr})
		}
	}
	c.mu.Lock()
	add("last", c.last[deviceID])
	c.mu.Unlock()
	if c.MDNS != nil {
		for _, a := range c.MDNS.Lookup(deviceID) {
			add("mdns", a)
		}
	}
	if c.Book != nil {
		if rec, ok, err := c.Book.AddressOf(ctx, deviceID); err == nil && ok {
			for _, a := range rec.Local {
				add("lan", a)
			}
			add("tailnet", rec.Tailnet)
			add("manual", rec.Manual)
		}
	}
	for _, a := range c.Manual[deviceID] {
		add("manual", a)
	}
	return out
}

// Dial probes the candidates in parallel and returns a client for the first
// that answers.
func (c *Connector) Dial(ctx context.Context, deviceID string) (*Client, Candidate, error) {
	cands := c.Candidates(ctx, deviceID)
	if len(cands) == 0 {
		return nil, Candidate{}, errors.New("peer: no known address")
	}
	type result struct {
		cl  *Client
		cd  Candidate
		err error
	}
	pctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch := make(chan result, len(cands))
	for _, cd := range cands {
		go func() {
			cl, err := NewClient(c.Identity, c.Group, cd.Addr, deviceID, c.Intervals, c.Presence)
			if err != nil {
				ch <- result{err: err}
				return
			}
			tctx, tcancel := context.WithTimeout(pctx, c.Intervals.probe())
			defer tcancel()
			_, err = cl.Vector(tctx)
			ch <- result{cl, cd, err}
		}()
	}
	var lastErr error
	for range cands {
		r := <-ch
		if r.err == nil {
			return r.cl, r.cd, nil
		}
		lastErr = r.err
	}
	return nil, Candidate{}, lastErr
}

// RunPeer syncs with one device until ctx ends, reconnecting with backoff.
func (c *Connector) RunPeer(ctx context.Context, deviceID string) {
	delay := c.Intervals.backoffMin()
	for ctx.Err() == nil {
		kick := c.kicked()
		cl, cd, err := c.Dial(ctx, deviceID)
		if err == nil {
			c.mu.Lock()
			if c.last == nil {
				c.last = map[string]string{}
			}
			c.last[deviceID] = cd.Addr
			c.mu.Unlock()
			if c.Presence != nil {
				c.Presence.Beat(deviceID)
			}
			if c.OnConnect != nil {
				c.OnConnect(deviceID, cd)
			}
			started := time.Now()
			_ = c.Engine.SyncWith(ctx, cl)
			if time.Since(started) > 2*c.Intervals.backoffMin() {
				delay = c.Intervals.backoffMin()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-kick:
			delay = c.Intervals.backoffMin()
			continue
		case <-time.After(delay):
		}
		delay = min(delay*2, c.Intervals.backoffMax())
	}
}

// Run starts RunPeer for every member but this device, following membership
// changes, until ctx ends.
func (c *Connector) Run(ctx context.Context) {
	every := c.ScanEvery
	if every <= 0 {
		every = 5 * time.Second
	}
	var wg sync.WaitGroup
	defer wg.Wait()
	c.mu.Lock()
	c.running = map[string]bool{}
	c.mu.Unlock()
	for {
		for _, m := range c.Group.Members() {
			if m.DeviceID == c.Identity.DeviceID() {
				continue
			}
			c.mu.Lock()
			run := c.running[m.DeviceID]
			c.running[m.DeviceID] = true
			c.mu.Unlock()
			if run {
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				c.RunPeer(ctx, m.DeviceID)
				c.mu.Lock()
				delete(c.running, m.DeviceID)
				c.mu.Unlock()
			}()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}
