package runtime

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/harness"
)

// slowDriver takes a while to detect and counts its detections.
type slowDriver struct {
	calls *atomic.Int32
	delay time.Duration
}

func (d slowDriver) Info() harness.Info {
	return harness.Info{ID: "slow", Driver: "slow", Name: "Slow"}
}
func (d slowDriver) Detect(context.Context) harness.Status {
	n := d.calls.Add(1)
	time.Sleep(d.delay)
	return harness.Status{Installed: true, Version: string(rune('0' + n))}
}
func (d slowDriver) Start(context.Context, harness.Turn) (harness.Session, error) { return nil, nil }

func TestProvidersServesStaleAndDetectsOnce(t *testing.T) {
	calls := &atomic.Int32{}
	m := NewManager(nil, nil, "n", t.TempDir(), testNodeState(t), nil, slowDriver{calls: calls, delay: 50 * time.Millisecond})
	changed := make(chan struct{}, 1)
	m.OnProvidersChange = func() { changed <- struct{}{} }

	// Concurrent first calls share one detection.
	done := make(chan []Status, 3)
	for range 3 {
		go func() { done <- m.Providers(context.Background()) }()
	}
	for range 3 {
		if got := <-done; len(got) != 1 || got[0].Version != "1" {
			t.Fatalf("first detection = %+v", got)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("detections = %d, want 1", calls.Load())
	}

	// A stale answer is served at once while a new detection runs.
	m.mu.Lock()
	m.detectedAt = time.Now().Add(-2 * providersFresh)
	m.mu.Unlock()
	start := time.Now()
	if got := m.Providers(context.Background()); got[0].Version != "1" || time.Since(start) > 20*time.Millisecond {
		t.Fatalf("stale call waited or changed: %+v", got)
	}
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("no change notice after the background detection")
	}
	if got := m.Providers(context.Background()); got[0].Version != "2" {
		t.Fatalf("after refresh = %+v", got)
	}
}
