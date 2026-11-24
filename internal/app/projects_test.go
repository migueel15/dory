package app

import (
	"path/filepath"
	"testing"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/oplog"
)

func TestSetTaskProject(t *testing.T) {
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
	projects := &Projects{Store: store}
	p, _ := store.Projects().Create(ctx, data.Project{Slug: "x", Name: "X"})
	task, _ := store.Tasks().Create(ctx, data.Task{Title: "Suelta"})
	got, err := projects.SetTaskProject(ctx, task.ID, "x")
	if err != nil || got.ProjectID == nil || *got.ProjectID != p.ID {
		t.Fatalf("link by slug: %+v, %v", got, err)
	}
	got, err = projects.SetTaskProject(ctx, task.ID, "")
	if err != nil || got.ProjectID != nil {
		t.Fatalf("unlink: %+v, %v", got, err)
	}
	if _, err = projects.SetTaskProject(ctx, task.ID, "nope"); err == nil {
		t.Fatal("unknown project accepted")
	}
}
