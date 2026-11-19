package api

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/oplog"
)

func TestPlanPending(t *testing.T) {
	ctx := context.Background()
	db, err := localdb.Open(ctx, filepath.Join(t.TempDir(), "dory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	log, err := oplog.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: data.New(db, log)}
	task, err := s.Store.Tasks().Put(ctx, data.Task{Title: "Tarea"})
	if err != nil {
		t.Fatal(err)
	}
	if pending, _ := s.planPending(ctx, task.ID); pending {
		t.Fatal("a task without a plan is pending")
	}
	if _, err = s.ProposePlan(ctx, task.ID, nil, nil, "agent"); err != nil {
		t.Fatal(err)
	}
	if pending, _ := s.planPending(ctx, task.ID); !pending {
		t.Fatal("a proposed plan is not pending")
	}
	if _, _, err = s.acceptPlan(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if pending, _ := s.planPending(ctx, task.ID); pending {
		t.Fatal("an accepted plan is still pending")
	}
}
