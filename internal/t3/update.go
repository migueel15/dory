package t3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/migueel15/dory/internal/service"
)

// updateTimeout covers an update: T3 downloads and installs before it answers.
const updateTimeout = 10 * time.Minute

// updatesTTL keeps the last read of what is outdated: it asks each T3 and the
// npm registry, so the page does not repeat it on every visit.
const updatesTTL = 5 * time.Minute

// Update results of one instance.
const (
	UpdateCurrent            = "current"
	UpdateDone               = "updated"
	UpdateNeedsConfirmation  = "needs_confirmation"
	UpdateFailed             = "failed"
	UpdateMissingScope       = "missing_scope"
	updateErrorUnavailable   = "unavailable"
	updateProviderBehind     = "behind_latest"
	updateActiveThreadStatus = "running"
)

// ServerUpdate is the T3 server of an instance and the release it could move to.
type ServerUpdate struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
}

// ProviderUpdate is one connector (Codex, Claude, ...) of an instance.
type ProviderUpdate struct {
	InstanceID string `json:"instance_id"`
	Driver     string `json:"driver"`
	Name       string `json:"name"`
	Current    string `json:"current"`
	Latest     string `json:"latest"`
	Available  bool   `json:"available"`
	// State is T3's own progress of an update it is running or ran: idle, queued,
	// running, succeeded, failed or unchanged.
	State   string `json:"state"`
	Message string `json:"message"`
}

// InstanceUpdates is what could be updated in one instance. It is read live
// and never stored.
type InstanceUpdates struct {
	InstanceID    string           `json:"instance_id"`
	EnvironmentID string           `json:"environment_id"`
	Server        ServerUpdate     `json:"server"`
	Providers     []ProviderUpdate `json:"providers"`
	// ActiveThreads run now; restarting the server would cut them.
	ActiveThreads int `json:"active_threads"`
	// Error is "unavailable" when the instance did not answer.
	Error string `json:"error,omitempty"`
}

// Pending reports whether anything of the instance can be updated.
func (u InstanceUpdates) Pending() bool {
	if u.Server.Available {
		return true
	}
	for _, p := range u.Providers {
		if p.Available {
			return true
		}
	}
	return false
}

// UpdateResult is the outcome of updating one instance.
type UpdateResult struct {
	InstanceID    string `json:"instance_id"`
	Status        string `json:"status"`
	ActiveThreads int    `json:"active_threads,omitempty"`
	Error         string `json:"error,omitempty"`
}

// releaseSpec is the npm release line an installed version follows.
func releaseSpec(version string) string {
	if strings.Contains(version, "nightly") {
		return "nightly"
	}
	return "latest"
}

type wireUpdateConfig struct {
	Providers []struct {
		InstanceID      string `json:"instanceId"`
		Driver          string `json:"driver"`
		DisplayName     string `json:"displayName"`
		VersionAdvisory *struct {
			Status         string `json:"status"`
			CurrentVersion string `json:"currentVersion"`
			LatestVersion  string `json:"latestVersion"`
			CanUpdate      bool   `json:"canUpdate"`
		} `json:"versionAdvisory"`
		UpdateState *struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"updateState"`
	} `json:"providers"`
}

// Updates reads the server version, the latest release and the connectors'
// version advisories of the instance.
func (c Client) Updates(ctx context.Context, current string) (ServerUpdate, []ProviderUpdate, error) {
	s := ServerUpdate{Current: current}
	providers, err := c.providerUpdates(ctx)
	if err != nil {
		return s, nil, err
	}
	var rel struct {
		Version string `json:"version"`
	}
	// The registry may be unreachable for T3; the connectors still count.
	if err := c.callFor(ctx, usageTimeout, "server.getNpmRelease", map[string]any{"spec": releaseSpec(current)}, &rel); err == nil && rel.Version != "" {
		s.Latest = rel.Version
		s.Available = current != "" && rel.Version != current
	}
	return s, providers, nil
}

// providerUpdates reads each connector's version advisory and update progress.
func (c Client) providerUpdates(ctx context.Context) ([]ProviderUpdate, error) {
	var cfg wireUpdateConfig
	if err := c.callFor(ctx, usageTimeout, "server.getConfig", map[string]any{}, &cfg); err != nil {
		return nil, err
	}
	providers := []ProviderUpdate{}
	for _, p := range cfg.Providers {
		if p.VersionAdvisory == nil {
			continue
		}
		u := ProviderUpdate{InstanceID: p.InstanceID, Driver: p.Driver, Name: p.DisplayName, Current: p.VersionAdvisory.CurrentVersion, Latest: p.VersionAdvisory.LatestVersion,
			Available: p.VersionAdvisory.Status == updateProviderBehind && p.VersionAdvisory.CanUpdate}
		if p.UpdateState != nil {
			u.State, u.Message = p.UpdateState.Status, p.UpdateState.Message
		}
		providers = append(providers, u)
	}
	return providers, nil
}

// UpdateProvider updates one connector to its latest version and waits for T3
// to finish: the call only queues the update, and restarting the server before
// it ends would cut it. T3 installs the latest release itself; asking for an
// exact version is refused by most installers.
func (c Client) UpdateProvider(ctx context.Context, p ProviderUpdate) error {
	var ignored map[string]any
	if err := c.callFor(ctx, updateTimeout, "server.updateProvider", map[string]any{"provider": p.Driver, "instanceId": p.InstanceID}, &ignored); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: the update did not finish in time", p.Name)
		case <-time.After(2 * time.Second):
		}
		list, err := c.providerUpdates(ctx)
		if err != nil {
			return err
		}
		for _, q := range list {
			if q.InstanceID != p.InstanceID {
				continue
			}
			switch q.State {
			case "succeeded", "unchanged":
				return nil
			case "failed":
				return errors.New(q.Message)
			}
		}
	}
}

// UpdateServer moves the T3 server to an exact release. T3 restarts itself, so
// the connection may close before it answers.
func (c Client) UpdateServer(ctx context.Context, target string) error {
	var ignored map[string]any
	return c.callFor(ctx, updateTimeout, "server.updateServer", map[string]any{"targetVersion": target}, &ignored)
}

// activeThreads counts the threads of an instance that are working now.
func activeThreads(threads []Thread) int {
	n := 0
	for _, t := range threads {
		switch t.Status {
		case updateActiveThreadStatus, "starting", "preparing":
			n++
		}
	}
	return n
}

type updatesCache struct {
	mu   sync.Mutex
	at   time.Time
	data []InstanceUpdates
}

// Updates lists what each instance can update. force skips the short cache.
func (m *Monitor) Updates(ctx context.Context, force bool) ([]InstanceUpdates, error) {
	m.updates.mu.Lock()
	defer m.updates.mu.Unlock()
	if !force && m.updates.data != nil && time.Since(m.updates.at) < updatesTTL {
		return m.withActive(m.updates.data), nil
	}
	instances, err := m.svc.T3Instances().List(ctx, service.Filter{Limit: 100})
	if err != nil {
		return nil, err
	}
	// A page reload cancels its request; the read must not turn that into
	// "unavailable" for everyone, so it runs on its own deadline.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*usageTimeout)
	defer cancel()
	out := make([]InstanceUpdates, len(instances))
	var wg sync.WaitGroup
	for i, in := range instances {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = m.readUpdates(ctx, in)
		}()
	}
	wg.Wait()
	// Only a complete read is kept: an instance that did not answer is asked again next time.
	failed := false
	for _, u := range out {
		failed = failed || u.Error != ""
	}
	if failed {
		m.updates.data = nil
	} else {
		m.updates.at, m.updates.data = time.Now(), out
	}
	return m.withActive(out), nil
}

func (m *Monitor) readUpdates(ctx context.Context, in service.T3Instance) InstanceUpdates {
	u := InstanceUpdates{InstanceID: in.ID, EnvironmentID: in.EnvironmentID, Providers: []ProviderUpdate{}}
	c := Client{BaseURL: in.BaseUrl, Token: os.Getenv(in.TokenEnv), HTTP: m.client}
	d, err := c.Discover(ctx)
	if err != nil {
		log.Printf("t3: updates of %s: discover: %v", in.BaseUrl, err)
		u.Error = updateErrorUnavailable
		return u
	}
	server, providers, err := c.Updates(ctx, d.ServerVersion)
	if err != nil {
		log.Printf("t3: updates of %s: %v", in.BaseUrl, err)
		u.Server = ServerUpdate{Current: d.ServerVersion}
		u.Error = updateErrorUnavailable
		return u
	}
	u.Server, u.Providers = server, providers
	return u
}

// withActive adds the running threads, which change faster than the cache.
func (m *Monitor) withActive(list []InstanceUpdates) []InstanceUpdates {
	out := make([]InstanceUpdates, len(list))
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i, u := range list {
		u.ActiveThreads = activeThreads(m.byID[u.InstanceID].Threads)
		out[i] = u
	}
	return out
}

// ApplyUpdates updates the connectors and then the server of one instance, or
// of all of them when instanceID is empty. An instance with running threads is
// not restarted unless restart is true; it is reported as needing confirmation.
func (m *Monitor) ApplyUpdates(ctx context.Context, instanceID string, restart bool) ([]UpdateResult, error) {
	list, err := m.Updates(ctx, true)
	if err != nil {
		return nil, err
	}
	var targets []InstanceUpdates
	for _, u := range list {
		if instanceID == "" || u.InstanceID == instanceID {
			targets = append(targets, u)
		}
	}
	if instanceID != "" && len(targets) == 0 {
		return nil, ErrInstanceUnavailable
	}
	out := make([]UpdateResult, len(targets))
	var wg sync.WaitGroup
	for i, u := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = m.applyOne(ctx, u, restart)
		}()
	}
	wg.Wait()
	m.updates.mu.Lock()
	m.updates.data = nil
	m.updates.mu.Unlock()
	return out, nil
}

func (m *Monitor) applyOne(ctx context.Context, u InstanceUpdates, restart bool) UpdateResult {
	r := UpdateResult{InstanceID: u.InstanceID, Status: UpdateCurrent, ActiveThreads: u.ActiveThreads}
	if u.Error != "" {
		r.Status, r.Error = UpdateFailed, u.Error
		return r
	}
	if u.Server.Available && u.ActiveThreads > 0 && !restart {
		r.Status = UpdateNeedsConfirmation
		return r
	}
	c, err := m.InstanceClient(ctx, u.InstanceID)
	if err != nil {
		r.Status, r.Error = UpdateFailed, err.Error()
		return r
	}
	ctx = context.WithoutCancel(ctx)
	fail := func(err error) UpdateResult {
		r.Status, r.Error = UpdateFailed, err.Error()
		if isMissingScope(err) {
			r.Status, r.Error = UpdateMissingScope, ""
		}
		return r
	}
	for _, p := range u.Providers {
		if !p.Available {
			continue
		}
		if err := c.UpdateProvider(ctx, p); err != nil {
			return fail(fmt.Errorf("%s: %w", p.Name, err))
		}
		r.Status = UpdateDone
	}
	if u.Server.Available {
		// A closed connection is how a restarting T3 answers; Refresh checks it came back.
		if err := c.UpdateServer(ctx, u.Server.Latest); err != nil && !isClosed(err) {
			return fail(err)
		}
		r.Status = UpdateDone
	}
	m.Refresh(u.InstanceID)
	return r
}

// isMissingScope reports T3 refusing the call because the token lacks a scope
// (environment:maintain for the server, providers:manage for a connector).
func isMissingScope(err error) bool {
	return errors.Is(err, errMissingScope) || strings.Contains(err.Error(), "EnvironmentAuthorizationError")
}

// isClosed reports a connection that ended without an answer.
func isClosed(err error) bool {
	return errors.Is(err, io.EOF) || websocket.CloseStatus(err) != -1
}
