package t3

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
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
}

func NewMonitor(svc service.Service) *Monitor {
	return &Monitor{svc: svc, client: &http.Client{Timeout: requestTimeout}, byID: map[string]Status{}}
}

// Run polls until ctx ends.
func (m *Monitor) Run(ctx context.Context) {
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
	case errors.Is(err, errUnauthorized):
		s.State = StateAuthFailed
	default:
		log.Printf("t3: threads of %s: %v", in.BaseUrl, err)
	}
	return s
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
