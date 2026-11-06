package t3

import (
	"context"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/service"
)

type countingInstances struct {
	service.Resource[service.T3Instance]
	lists int
	err   error
}

func (c *countingInstances) List(ctx context.Context, _ service.Filter) ([]service.T3Instance, error) {
	c.lists++
	c.err = ctx.Err()
	return nil, nil
}

type instancesService struct {
	service.Service
	instances *countingInstances
}

func (s instancesService) T3Instances() service.Resource[service.T3Instance] { return s.instances }

func TestRefreshUsageSharesRecentRead(t *testing.T) {
	instances := &countingInstances{}
	m := NewMonitor(instancesService{instances: instances})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m.RefreshUsage(ctx)
	m.RefreshUsage(context.Background())
	if instances.lists != 1 {
		t.Fatalf("reads = %d", instances.lists)
	}
	if instances.err != nil {
		t.Fatalf("read got cancelled: %v", instances.err)
	}
}

func TestMonitorSubscribeSignalsChanges(t *testing.T) {
	m := NewMonitor(instancesService{})
	ch, cancel := m.Subscribe()
	defer cancel()
	s := Status{InstanceID: "a", State: StateAvailable, Threads: []Thread{{ID: "t", Status: "running"}}}
	m.store(s)
	select {
	case <-ch:
	default:
		t.Fatal("first status did not signal")
	}
	now := time.Now()
	s.LastSeen = &now
	m.store(s)
	select {
	case <-ch:
		t.Fatal("LastSeen alone must not signal")
	default:
	}
	s.Threads = []Thread{{ID: "t", Status: "ready"}}
	m.store(s)
	select {
	case <-ch:
	default:
		t.Fatal("thread change did not signal")
	}
}
