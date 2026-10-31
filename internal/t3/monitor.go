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

const pollInterval = 10 * time.Second

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
	svc    service.Service
	client *http.Client
	mu     sync.RWMutex
	byID   map[string]Status
	usage  map[string]InstanceUsage
}

func NewMonitor(svc service.Service) *Monitor {
	return &Monitor{svc: svc, client: &http.Client{Timeout: requestTimeout}, byID: map[string]Status{}, usage: map[string]InstanceUsage{}}
}

// Run polls until ctx ends.
func (m *Monitor) Run(ctx context.Context) {
	go m.runUsage(ctx)
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
	var wg sync.WaitGroup
	for _, in := range instances {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := m.check(ctx, in)
			m.mu.Lock()
			defer m.mu.Unlock()
			m.byID[in.ID] = s
		}()
	}
	wg.Wait()
	current := make(map[string]bool, len(instances))
	for _, in := range instances {
		current[in.ID] = true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.byID {
		if !current[id] {
			delete(m.byID, id)
		}
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
			s.Threads[i].URL = strings.TrimRight(in.BaseUrl, "/") + "/" + d.EnvironmentID + "/" + s.Threads[i].ID
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
		m.pollUsage(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (m *Monitor) pollUsage(ctx context.Context) {
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
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usage = next
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
