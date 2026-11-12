package localstore

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/service"
)

func TestSyncHooksAndReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "dory.db")
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	s, err := Open(path, Options{Device: "dev-a", Local: true, Clock: func() time.Time { return at }})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Projects().Put(ctx, "test", service.Project{Slug: "hooks", Name: "N"})
	if err != nil {
		t.Fatal(err)
	}
	at = at.Add(time.Minute)
	p.Name = "M"
	if _, err = s.Projects().Put(ctx, "test", p); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s, err = Open(path, Options{}) // reopening applies no migration twice
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if g, err := s.Projects().Get(ctx, p.ID); err != nil || g.Name != "M" {
		t.Fatalf("reopened = %+v, %v", g, err)
	}
	var owner, clock string
	if err = s.root.QueryRow(`SELECT owner_device, field_clock FROM records WHERE id = ?`, p.ID).Scan(&owner, &clock); err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	json.Unmarshal([]byte(clock), &m)
	if owner != "dev-a" || m["name"] != "2026-01-02T03:05:05.000000Z@dev-a" || m["slug"] != "2026-01-02T03:04:05.000000Z@dev-a" {
		t.Fatalf("owner=%q clock=%v", owner, m)
	}
}
