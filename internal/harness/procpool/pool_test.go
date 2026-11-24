package procpool

import (
	"testing"
	"time"
)

type testProcess struct {
	busy, closed bool
	idle         time.Time
}

func (p *testProcess) Busy() bool           { return p.busy }
func (p *testProcess) IdleSince() time.Time { return p.idle }
func (p *testProcess) Close()               { p.closed = true }

func TestReap(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p := New(func(thread string) bool { return thread == "service" })
	p.now = func() time.Time { return now }
	stale := now.Add(-IdleTimeout - time.Second)
	idle, busy := &testProcess{idle: stale}, &testProcess{busy: true, idle: stale}
	service, recent := &testProcess{idle: stale}, &testProcess{idle: now.Add(-IdleTimeout)}
	p.Set("idle", "driver", idle)
	p.Set("busy", "driver", busy)
	p.Set("service", "driver", service)
	p.Set("recent", "driver", recent)
	p.reap()
	if !idle.closed || p.Get("idle", "driver") != nil || busy.closed || service.closed || recent.closed {
		t.Fatal("expected only the expired process without services to be reaped")
	}
	p.CloseThread("service")
	if !service.closed || p.Get("service", "driver") != nil {
		t.Fatal("expected the thread process to close")
	}
	p.CloseAll()
	if !busy.closed || !recent.closed {
		t.Fatal("expected the remaining processes to close")
	}
}

func TestPoolIsolation(t *testing.T) {
	a, b := New(nil), New(nil)
	first, second := &testProcess{}, &testProcess{}
	a.Set("thread", "driver", first)
	b.Set("thread", "driver", second)
	a.Drop("thread", "driver", second)
	if a.Get("thread", "driver") != first {
		t.Fatal("a stale process removed the current process")
	}
	a.CloseAll()
	if !first.closed || second.closed {
		t.Fatal("closing a pool affected a second pool")
	}
}
