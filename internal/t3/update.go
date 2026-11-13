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

// ServerUpdating is the state of a server being replaced.
const ServerUpdating = "updating"

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
	// State is "updating" while Dory replaces the server.
	State string `json:"state,omitempty"`
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
	// Updating while Dory runs an update of this instance.
	Updating bool `json:"updating,omitempty"`
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
func (c Client) UpdateProvider(ctx context.Context, p ProviderUpdate, progress func()) error {
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
		if progress != nil {
			progress()
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
		// A thread with a turn queued or waiting for the user would lose it too.
		case updateActiveThreadStatus, "starting", "preparing", "queued", "waiting":
			n++
		}
	}
	return n
}

// updatesState is what Dory keeps about updates: the npm lookups (slow and
// rarely changing) and the instances whose server is being replaced now.
type updatesState struct {
	mu      sync.Mutex
	release map[string]cachedRelease
	busy    map[string]string // instance -> "providers" or "server" while Dory updates it
}

type cachedRelease struct {
	at      time.Time
	spec    string
	version string
}

// setPhase records what Dory is updating in an instance ("" when done) and tells watchers.
func (m *Monitor) setPhase(id, phase string) {
	m.updates.mu.Lock()
	if m.updates.busy == nil {
		m.updates.busy = map[string]string{}
	}
	if phase == "" {
		delete(m.updates.busy, id)
	} else {
		m.updates.busy[id] = phase
	}
	m.updates.mu.Unlock()
	m.notify()
}

func (m *Monitor) phase(id string) string {
	m.updates.mu.Lock()
	defer m.updates.mu.Unlock()
	return m.updates.busy[id]
}

// latest is the newest release of the line the installed version follows,
// asked to the instance at most every updatesTTL.
func (m *Monitor) latest(ctx context.Context, id string, c Client, current string) string {
	spec := releaseSpec(current)
	m.updates.mu.Lock()
	r, ok := m.updates.release[id]
	m.updates.mu.Unlock()
	if ok && r.spec == spec && time.Since(r.at) < updatesTTL {
		return r.version
	}
	var rel struct {
		Version string `json:"version"`
	}
	if err := c.callFor(ctx, usageTimeout, "server.getNpmRelease", map[string]any{"spec": spec}, &rel); err != nil || rel.Version == "" {
		return r.version // the registry may be unreachable for T3; keep what was known
	}
	m.updates.mu.Lock()
	if m.updates.release == nil {
		m.updates.release = map[string]cachedRelease{}
	}
	m.updates.release[id] = cachedRelease{at: time.Now(), spec: spec, version: rel.Version}
	m.updates.mu.Unlock()
	return rel.Version
}

// Updates lists what each instance can update, read live from the instances
// each time (a connector's progress changes second by second); only the npm
// release lookup is remembered.
func (m *Monitor) Updates(ctx context.Context) ([]InstanceUpdates, error) {
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
	return m.withActive(out), nil
}

func (m *Monitor) readUpdates(ctx context.Context, in service.T3Instance) InstanceUpdates {
	u := InstanceUpdates{InstanceID: in.ID, EnvironmentID: in.EnvironmentID, Providers: []ProviderUpdate{}}
	c := Client{BaseURL: in.BaseUrl, Token: os.Getenv(in.TokenEnv), HTTP: m.client}
	phase := m.phase(in.ID)
	u.Updating = phase != ""
	d, err := c.Discover(ctx)
	if err != nil {
		// A server being replaced does not answer for a moment: that is the update, not a fault.
		if phase == "server" {
			u.Server = ServerUpdate{Available: true, State: ServerUpdating}
			return u
		}
		log.Printf("t3: updates of %s: discover: %v", in.BaseUrl, err)
		u.Error = updateErrorUnavailable
		return u
	}
	providers, err := c.providerUpdates(ctx)
	if err != nil {
		log.Printf("t3: updates of %s: %v", in.BaseUrl, err)
		u.Server = ServerUpdate{Current: d.ServerVersion}
		u.Error = updateErrorUnavailable
		return u
	}
	u.Providers = providers
	u.Server = ServerUpdate{Current: d.ServerVersion, Latest: m.latest(ctx, in.ID, c, d.ServerVersion)}
	u.Server.Available = u.Server.Latest != "" && u.Server.Latest != d.ServerVersion
	if phase == "server" {
		u.Server.State = ServerUpdating
	}
	return u
}

// withActive adds the running threads, which change faster than the updates.
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
	list, err := m.Updates(ctx)
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
	return out, nil
}

func (m *Monitor) applyOne(ctx context.Context, u InstanceUpdates, restart bool) UpdateResult {
	r := UpdateResult{InstanceID: u.InstanceID, Status: UpdateCurrent, ActiveThreads: u.ActiveThreads}
	if u.Error != "" {
		r.Status, r.Error = UpdateFailed, u.Error
		return r
	}
	if u.Updating {
		return r // already being updated
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
	m.setPhase(u.InstanceID, "providers")
	defer m.setPhase(u.InstanceID, "")
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
		if err := c.UpdateProvider(ctx, p, m.notify); err != nil {
			return fail(fmt.Errorf("%s: %w", p.Name, err))
		}
		r.Status = UpdateDone
	}
	if u.Server.Available {
		m.setPhase(u.InstanceID, "server")
		// A closed connection is how a restarting T3 answers; below, the new version is waited for.
		if err := c.UpdateServer(ctx, u.Server.Latest); err != nil && !isClosed(err) {
			return fail(err)
		}
		c.waitVersion(ctx, u.Server.Current)
		r.Status = UpdateDone
	}
	m.Refresh(u.InstanceID)
	m.notify()
	return r
}

// waitVersion waits until the instance answers with a version other than old,
// that is, until the restart after an update has finished.
func (c Client) waitVersion(ctx context.Context, old string) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
		if d, err := c.Discover(ctx); err == nil && d.ServerVersion != old {
			return
		}
	}
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
