package t3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/service"
)

// pollInterval checks instances without a live shell stream; with one, a
// check runs at least every fallbackInterval in case a signal was missed.
const (
	pollInterval     = 10 * time.Second
	fallbackInterval = 60 * time.Second
	streamDebounce   = 200 * time.Millisecond
	backoffMin       = time.Second
	backoffMax       = 30 * time.Second
)

// Instance states shown to the user.
const (
	StateAvailable    = "available"
	StateDisconnected = "disconnected"
	StateAuthFailed   = "auth_failed"
	StateIncompatible = "incompatible"
)

// Status is the live view of one instance. It is never stored.
type Status struct {
	InstanceID    string     `json:"instance_id"`
	EnvironmentID string     `json:"environment_id"`
	BaseURL       string     `json:"base_url"`
	State         string     `json:"state"`
	Version       string     `json:"version"`
	LastSeen      *time.Time `json:"last_seen"`
	Threads       []Thread   `json:"threads"`
}

// Monitor polls every registered instance and keeps the latest status.
type Monitor struct {
	Observe func(context.Context, string, []Thread) error
	svc     service.Service
	client  *http.Client
	mu      sync.RWMutex
	byID    map[string]Status
	usage   map[string]InstanceUsage
	// polling keeps usage reads in order, so a slow read cannot replace a newer one.
	polling   sync.Mutex
	refreshed time.Time
	subMu     sync.Mutex
	subs      map[chan struct{}]struct{}
	// watchers holds the shell stream of each instance; live marks the ones
	// whose stream is connected right now.
	watchMu  sync.Mutex
	watchers map[string]context.CancelFunc
	live     map[string]bool
}

func NewMonitor(svc service.Service) *Monitor {
	return &Monitor{svc: svc, client: &http.Client{Timeout: requestTimeout}, byID: map[string]Status{}, usage: map[string]InstanceUsage{}, subs: map[chan struct{}]struct{}{}, watchers: map[string]context.CancelFunc{}, live: map[string]bool{}}
}

// Run polls until ctx ends.
func (m *Monitor) Run(ctx context.Context) {
	go m.runUsage(ctx)
	go m.runDiscovery(ctx)
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	for {
		m.poll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (m *Monitor) poll(ctx context.Context) {
	instances, err := m.svc.T3Instances().List(ctx, service.Filter{Limit: 100})
	if err != nil {
		log.Printf("t3: list instances: %v", err)
		return
	}
	m.watch(ctx, instances)
	var wg sync.WaitGroup
	for _, in := range instances {
		if m.streaming(in.ID) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.store(m.check(ctx, in))
		}()
	}
	wg.Wait()
	current := make(map[string]bool, len(instances))
	for _, in := range instances {
		current[in.ID] = true
	}
	m.mu.Lock()
	removed := false
	for id := range m.byID {
		if !current[id] {
			delete(m.byID, id)
			removed = true
		}
	}
	m.mu.Unlock()
	if removed {
		m.notify()
	}
}

// store saves a status and signals subscribers when what the SPA shows changed.
func (m *Monitor) store(s Status) {
	m.mu.Lock()
	old, had := m.byID[s.InstanceID]
	m.byID[s.InstanceID] = s
	m.mu.Unlock()
	if !had || !sameView(old, s) {
		m.notify()
	}
}

// sameView ignores LastSeen, which changes on every successful read.
func sameView(a, b Status) bool {
	a.LastSeen, b.LastSeen = nil, nil
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// Subscribe returns a channel that receives a signal after each change of the
// statuses. Signals coalesce: a slow reader gets one for a burst. cancel frees it.
func (m *Monitor) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	m.subMu.Lock()
	m.subs[ch] = struct{}{}
	m.subMu.Unlock()
	return ch, func() {
		m.subMu.Lock()
		delete(m.subs, ch)
		m.subMu.Unlock()
	}
}

func (m *Monitor) notify() {
	m.subMu.Lock()
	defer m.subMu.Unlock()
	for ch := range m.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// refreshDelays re-read an instance after a write: right away, and again
// shortly after because T3 applies some commands asynchronously.
var refreshDelays = []time.Duration{0, time.Second, 3 * time.Second}

// Refresh re-reads one instance in the background after Dory wrote to it, so
// the views do not wait for the next poll.
func (m *Monitor) Refresh(instanceID string) {
	go func() {
		for i, d := range refreshDelays {
			if i > 0 {
				time.Sleep(d - refreshDelays[i-1])
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*requestTimeout)
			in, err := m.svc.T3Instances().Get(ctx, instanceID)
			if err == nil {
				m.store(m.check(ctx, in))
			}
			cancel()
			if err != nil {
				log.Printf("t3: refresh %s: %v", instanceID, err)
				return
			}
		}
	}()
}

// RefreshThread refreshes the instance that reports a thread, if any.
func (m *Monitor) RefreshThread(threadID string) {
	if id, ok := m.Locate(threadID); ok {
		m.Refresh(id)
	}
}

func (m *Monitor) check(ctx context.Context, in service.T3Instance) Status {
	s := Status{InstanceID: in.ID, EnvironmentID: in.EnvironmentID, BaseURL: in.BaseUrl, State: StateDisconnected, Threads: []Thread{}}
	m.mu.RLock()
	s.LastSeen = m.byID[in.ID].LastSeen
	m.mu.RUnlock()
	c := Client{BaseURL: in.BaseUrl, Token: os.Getenv(in.TokenEnv), HTTP: m.client}
	d, err := c.Discover(ctx)
	s.Version = d.ServerVersion
	if errors.Is(err, errIncompatible) {
		s.State = StateIncompatible
		return s
	}
	if err != nil {
		return s
	}
	now := time.Now()
	s.LastSeen = &now
	threads, err := c.Threads(ctx)
	switch {
	case err == nil:
		s.State, s.Threads = StateAvailable, threads
		for i := range s.Threads {
			s.Threads[i].EnvironmentID = d.EnvironmentID
			s.Threads[i].URL = strings.TrimRight(in.BaseUrl, "/") + "/" + d.EnvironmentID + "/" + s.Threads[i].ID
		}
		if m.Observe != nil {
			all, err := c.threads(ctx, true)
			if err == nil {
				for i := range all {
					all[i].URL = strings.TrimRight(in.BaseUrl, "/") + "/" + d.EnvironmentID + "/" + all[i].ID
				}
				err = m.Observe(ctx, in.ID, all)
			}
			if err != nil {
				log.Printf("t3: notifications: %v", err)
			}
		}
	case errors.Is(err, errUnauthorized):
		s.State = StateAuthFailed
	default:
		log.Printf("t3: threads of %s: %v", in.BaseUrl, err)
	}
	return s
}

func (m *Monitor) runUsage(ctx context.Context) {
	tick := time.NewTicker(usageInterval)
	defer tick.Stop()
	for {
		m.pollUsage(ctx, false)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (m *Monitor) pollUsage(ctx context.Context, refresh bool) {
	m.polling.Lock()
	defer m.polling.Unlock()
	if refresh && time.Since(m.refreshed) < usageRefreshGap {
		return
	}
	instances, err := m.svc.T3Instances().List(ctx, service.Filter{Limit: 100})
	if err != nil {
		log.Printf("t3: list instances: %v", err)
		return
	}
	next := make(map[string]InstanceUsage, len(instances))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, in := range instances {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := Client{BaseURL: in.BaseUrl, Token: os.Getenv(in.TokenEnv), HTTP: m.client}
			if refresh {
				if err := c.RefreshProviders(ctx); err != nil {
					log.Printf("t3: refresh providers of %s: %v", in.BaseUrl, err)
				}
			}
			u, err := c.Usage(ctx, time.Now())
			if err != nil {
				u.LimitsError, u.ModelsError = UsageUnavailable, UsageUnavailable
			}
			u.InstanceID, u.EnvironmentID = in.ID, in.EnvironmentID
			mu.Lock()
			defer mu.Unlock()
			next[in.ID] = u
		}()
	}
	wg.Wait()
	if refresh {
		m.refreshed = time.Now()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usage = next
}

// RefreshUsage forces a new read and returns it. Calls that overlap or follow
// one within usageRefreshGap share its result. A reload cancels the request, but
// must not store its aborted read as an unavailable instance.
func (m *Monitor) RefreshUsage(ctx context.Context) []InstanceUsage {
	m.pollUsage(context.WithoutCancel(ctx), true)
	return m.Usage()
}

// UsageHistory reads the daily usage of every instance over the last days.
func (m *Monitor) UsageHistory(ctx context.Context, days int) ([]InstanceHistory, error) {
	days = max(1, min(days, MaxHistoryDays))
	instances, err := m.svc.T3Instances().List(ctx, service.Filter{Limit: 100})
	if err != nil {
		return nil, err
	}
	out := make([]InstanceHistory, len(instances))
	var wg sync.WaitGroup
	for i, in := range instances {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := Client{BaseURL: in.BaseUrl, Token: os.Getenv(in.TokenEnv), HTTP: m.client}
			h := InstanceHistory{InstanceID: in.ID, EnvironmentID: in.EnvironmentID, Days: []DayUsage{}}
			d, err := c.UsageHistory(ctx, time.Now(), days)
			switch {
			case err == nil:
				h.Days = d
			case errors.Is(err, errMissingScope):
				h.Error = UsageMissingScope
			default:
				h.Error = UsageUnavailable
			}
			out[i] = h
		}()
	}
	wg.Wait()
	return out, nil
}

// Usage returns the latest usage read of every instance.
func (m *Monitor) Usage() []InstanceUsage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]InstanceUsage, 0, len(m.usage))
	for _, u := range m.usage {
		out = append(out, u)
	}
	return out
}

// Locate finds the instance that currently reports a thread.
func (m *Monitor) Locate(threadID string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for id, s := range m.byID {
		for _, t := range s.Threads {
			if t.ID == threadID {
				return id, true
			}
		}
	}
	return "", false
}

// Thread returns the live view of a thread reported by an instance.
func (m *Monitor) Thread(threadID string) (Thread, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.byID {
		for _, t := range s.Threads {
			if t.ID == threadID {
				return t, true
			}
		}
	}
	return Thread{}, false
}

// ThreadClient returns a client for the instance that reports a thread, so
// writes can only reach threads Dory is already showing.
func (m *Monitor) ThreadClient(ctx context.Context, threadID string) (Client, error) {
	id, ok := m.Locate(threadID)
	if !ok {
		return Client{}, ErrThreadNotFound
	}
	in, err := m.svc.T3Instances().Get(ctx, id)
	if err != nil {
		return Client{}, err
	}
	return Client{BaseURL: in.BaseUrl, Token: os.Getenv(in.TokenEnv), HTTP: m.client}, nil
}

// InstanceClient returns a client for an instance that is available right now.
func (m *Monitor) InstanceClient(ctx context.Context, id string) (Client, error) {
	m.mu.RLock()
	state := m.byID[id].State
	m.mu.RUnlock()
	if state != StateAvailable {
		return Client{}, ErrInstanceUnavailable
	}
	in, err := m.svc.T3Instances().Get(ctx, id)
	if err != nil {
		return Client{}, err
	}
	return Client{BaseURL: in.BaseUrl, Token: os.Getenv(in.TokenEnv), HTTP: m.client}, nil
}

// PairingLink mints a one-time sign-in link. It runs the t3 CLI, so it only
// works for the instance on this host (DORY_T3_LOCAL_URL); the CLI grants the
// default scopes without storing a full access token in Dory.
func (m *Monitor) PairingLink(ctx context.Context, instanceID string) (string, error) {
	in, err := m.svc.T3Instances().Get(ctx, instanceID)
	if err != nil {
		return "", err
	}
	if local := os.Getenv("DORY_T3_LOCAL_URL"); local == "" || strings.TrimRight(local, "/") != strings.TrimRight(in.BaseUrl, "/") {
		return "", errors.New("t3: pairing is only available for the local instance")
	}
	bin := os.Getenv("DORY_T3_BIN")
	if bin == "" {
		bin = "t3"
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, bin, "auth", "pairing", "create", "--ttl", "5m", "--label", "dory", "--json").Output()
	if err != nil {
		return "", fmt.Errorf("t3: pairing command: %w", err)
	}
	var out struct {
		Credential string `json:"credential"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.Credential == "" {
		return "", errors.New("t3: pairing command returned no credential")
	}
	return strings.TrimRight(in.BaseUrl, "/") + "/pair#token=" + url.QueryEscape(out.Credential), nil
}

// Statuses returns the latest known status of every instance.
func (m *Monitor) Statuses() []Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Status, 0, len(m.byID))
	for _, s := range m.byID {
		out = append(out, s)
	}
	return out
}

// streaming reports whether an instance has a live shell stream that was
// checked recently, so the poll can skip it.
func (m *Monitor) streaming(id string) bool {
	m.watchMu.Lock()
	live := m.live[id]
	m.watchMu.Unlock()
	m.mu.RLock()
	seen := m.byID[id].LastSeen
	m.mu.RUnlock()
	return live && seen != nil && time.Since(*seen) < fallbackInterval
}

// watch keeps one shell stream per registered instance and stops the ones of
// removed instances.
func (m *Monitor) watch(ctx context.Context, instances []service.T3Instance) {
	m.watchMu.Lock()
	defer m.watchMu.Unlock()
	current := make(map[string]bool, len(instances))
	for _, in := range instances {
		current[in.ID] = true
		if _, ok := m.watchers[in.ID]; ok {
			continue
		}
		wctx, cancel := context.WithCancel(ctx)
		m.watchers[in.ID] = cancel
		go m.follow(wctx, in)
	}
	for id, cancel := range m.watchers {
		if !current[id] {
			cancel()
			delete(m.watchers, id)
			delete(m.live, id)
		}
	}
}

func (m *Monitor) setLive(id string, v bool) {
	m.watchMu.Lock()
	defer m.watchMu.Unlock()
	if _, ok := m.watchers[id]; ok {
		m.live[id] = v
	}
}

// follow re-reads an instance after each burst of shell changes. When the
// stream fails it retries with a growing delay, and the poll covers the gap.
func (m *Monitor) follow(ctx context.Context, in service.T3Instance) {
	signal := make(chan struct{}, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-signal:
			}
			// Coalesce the burst a running turn produces into one read.
			select {
			case <-ctx.Done():
				return
			case <-time.After(streamDebounce):
			}
			select {
			case <-signal:
			default:
			}
			rctx, cancel := context.WithTimeout(ctx, 2*requestTimeout)
			m.store(m.check(rctx, in))
			cancel()
		}
	}()
	delay := backoffMin
	for {
		c := Client{BaseURL: in.BaseUrl, Token: os.Getenv(in.TokenEnv), HTTP: m.client}
		err := c.Stream(ctx, methodSubscribeShell, shellPayload, func() {
			m.setLive(in.ID, true)
			delay = backoffMin
			select {
			case signal <- struct{}{}:
			default:
			}
		})
		m.setLive(in.ID, false)
		if ctx.Err() != nil {
			return
		}
		log.Printf("t3: shell stream of %s: %v; retrying in %s", in.BaseUrl, err, delay)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(2*delay, backoffMax)
	}
}

// FollowThread sends on signal (without blocking, so bursts coalesce) after
// each change of one thread, until ctx ends or the stream fails. c is the
// client that reaches the thread.
func FollowThread(ctx context.Context, c Client, threadID string, signal chan<- struct{}) error {
	first := true
	return c.Stream(ctx, methodSubscribeThread, map[string]any{"threadId": threadID}, func() {
		// The first chunk is the snapshot the view already has.
		if first {
			first = false
			return
		}
		select {
		case signal <- struct{}{}:
		default:
		}
	})
}
