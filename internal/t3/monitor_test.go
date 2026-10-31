package t3

import (
	"context"
	"testing"

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
