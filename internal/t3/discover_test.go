package t3

import (
	"context"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/service"
)

type fakeResource[T any] struct {
	service.Resource[T]
	rows []T
	set  func(*T, string)
}

func (f *fakeResource[T]) List(context.Context, service.Filter) ([]T, error) { return f.rows, nil }
func (f *fakeResource[T]) Put(_ context.Context, _ string, v T) (T, error) {
	f.set(&v, "new-env")
	f.rows = append(f.rows, v)
	return v, nil
}

type discoverService struct {
	service.Service
	envs      *fakeResource[service.Environment]
	instances *fakeResource[service.T3Instance]
}

func (s discoverService) Environments() service.Resource[service.Environment] { return s.envs }
func (s discoverService) T3Instances() service.Resource[service.T3Instance]   { return s.instances }

func newDiscoverService(envs ...service.Environment) discoverService {
	return discoverService{
		envs:      &fakeResource[service.Environment]{rows: envs, set: func(e *service.Environment, id string) { e.ID = id }},
		instances: &fakeResource[service.T3Instance]{set: func(*service.T3Instance, string) {}},
	}
}

func TestScanHosts(t *testing.T) {
	hosts, err := scanHosts("192.168.1.7/24")
	if err != nil || len(hosts) != 254 || hosts[0].String() != "192.168.1.1" {
		t.Fatalf("hosts=%d err=%v", len(hosts), err)
	}
	if _, err := scanHosts("10.0.0.0/8"); err == nil {
		t.Fatal("wide range accepted")
	}
}

func TestSyncFoundCreatesEnvironmentAndInstance(t *testing.T) {
	svc := newDiscoverService()
	d := Discovery{EnvironmentID: "a", Label: "arch-linux"}
	d.Platform.OS = "linux"
	if err := NewMonitor(svc).syncFound(context.Background(), map[string]Discovery{"http://h:3773": d}); err != nil {
		t.Fatal(err)
	}
	if len(svc.envs.rows) != 1 || svc.envs.rows[0].Os != "linux" || len(svc.instances.rows) != 1 {
		t.Fatalf("envs=%+v instances=%+v", svc.envs.rows, svc.instances.rows)
	}
	if in := svc.instances.rows[0]; in.EnvironmentID != "new-env" || in.TokenEnv != "T3_TOKEN_ARCH_LINUX" {
		t.Fatalf("instance=%+v", in)
	}
}

func TestSyncFoundSkipsArchivedEnvironment(t *testing.T) {
	now := time.Now()
	archived := service.Environment{Name: "arch"}
	archived.ArchivedAt = &now
	svc := newDiscoverService(archived)
	found := map[string]Discovery{"http://h:3773": {EnvironmentID: "a", Label: "arch"}}
	if err := NewMonitor(svc).syncFound(context.Background(), found); err != nil {
		t.Fatal(err)
	}
	if len(svc.instances.rows) != 0 {
		t.Fatalf("instances=%+v", svc.instances.rows)
	}
}
