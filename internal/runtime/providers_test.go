package runtime

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/harness"
)

type slowDetectProvider struct {
	calls   atomic.Int32
	blockCh chan struct{}
}

func (p *slowDetectProvider) Info() harness.Info {
	return harness.Info{ID: "slow", Driver: "slow", Name: "Slow"}
}

func (p *slowDetectProvider) Detect(ctx context.Context) Status {
	n := p.calls.Add(1)
	if n == 2 && p.blockCh != nil {
		select {
		case <-p.blockCh:
		case <-ctx.Done():
		}
	}
	return Status{
		Installed: true,
		Version:   time.Duration(n).String(),
	}
}

func (p *slowDetectProvider) Start(context.Context, harness.Turn) (harness.Session, error) {
	return nil, nil
}

func TestProvidersStaleWhileRevalidateAndInvalidate(t *testing.T) {
	state := testNodeState(t)

	block := make(chan struct{})
	prov := &slowDetectProvider{blockCh: block}
	m := NewManager(nil, nil, "n1", t.TempDir(), state, nil, prov)

	// Cold call detects synchronously.
	first := m.Providers(context.Background())
	if len(first) != 1 || first[0].Version != "1ns" {
		t.Fatalf("cold Providers = %+v", first)
	}

	// Mark cache stale; next call must return immediately with stale data and
	// trigger only one background refresh.
	m.mu.Lock()
	m.detectedAt = time.Now().Add(-2 * time.Minute)
	m.mu.Unlock()

	start := time.Now()
	stale1 := m.Providers(context.Background())
	stale2 := m.Providers(context.Background())
	if time.Since(start) > 200*time.Millisecond {
		t.Fatalf("stale Providers blocked for %v", time.Since(start))
	}
	if len(stale1) != 1 || stale1[0].Version != "1ns" || stale2[0].Version != "1ns" {
		t.Fatalf("expected stale 1ns, got %+v / %+v", stale1, stale2)
	}

	// Unblock background refresh and wait for cache update.
	close(block)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got := m.Providers(context.Background())
		if len(got) == 1 && got[0].Version == "2ns" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := m.Providers(context.Background()); got[0].Version != "2ns" {
		t.Fatalf("expected refreshed 2ns, got %+v", got)
	}
	if prov.calls.Load() != 2 {
		t.Fatalf("expected exactly 2 Detect calls, got %d", prov.calls.Load())
	}

	// Explicit invalidate forces synchronous detection on next call.
	m.invalidate()
	after := m.Providers(context.Background())
	if len(after) != 1 || after[0].Version != "3ns" {
		t.Fatalf("after invalidate expected 3ns, got %+v", after)
	}
}
