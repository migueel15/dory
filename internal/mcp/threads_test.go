package mcp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/api"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/devices"
	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/oplog"
)

func TestThreadControlTools(t *testing.T) {
	tools := toolsOf(t, build(&api.Server{Devices: devices.NewManager()}, false))
	for _, n := range []string{"list_models", "list_threads", "get_thread", "thread_configure", "thread_send", "thread_stop", "thread_spawn", "thread_create", "list_environments", "subagent_status", "subagent_cancel"} {
		if tools[n] == nil {
			t.Errorf("missing %s", n)
		}
	}
	if !strings.Contains(tools["thread_configure"].Description, "continue_with") {
		t.Error("thread_configure does not explain continue_with")
	}
}

func TestTargetDefaultsToCaller(t *testing.T) {
	ctx := context.WithValue(context.Background(), threadKey{}, "self")
	if id, _ := target(ctx, ""); id != "self" {
		t.Fatalf("default target %q", id)
	}
	if id, _ := target(ctx, "other"); id != "other" {
		t.Fatalf("explicit target %q", id)
	}
	if _, err := target(context.Background(), ""); err == nil {
		t.Fatal("no thread accepted")
	}
}

func TestFindProjectAndEnvironment(t *testing.T) {
	ctx := t.Context()
	db, err := localdb.Open(ctx, filepath.Join(t.TempDir(), "dory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	log, err := oplog.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	store := data.New(db, log)
	p, _ := store.Projects().Put(ctx, data.Project{Slug: "dory", Name: "Dory"})
	for _, name := range []string{p.ID, "dory", "DORY"} {
		if got, err := findProjectByName(ctx, store, name); err != nil || got.ID != p.ID {
			t.Fatalf("project %q: %+v %v", name, got, err)
		}
	}
	if _, err := findProjectByName(ctx, store, "nope"); err == nil {
		t.Fatal("unknown project accepted")
	}
	s := &api.Server{Node: api.Node{ID: "n1", Name: "Portátil"}}
	for _, name := range []string{"", "local", "n1", "portátil"} {
		if got, err := findEnvironment(ctx, s, name); err != nil || got != "n1" {
			t.Fatalf("environment %q: %q %v", name, got, err)
		}
	}
	if _, err := findEnvironment(ctx, s, "servidor"); err == nil {
		t.Fatal("unknown environment accepted")
	}
}
