package api

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/oplog"
)

func newPlanServer(t *testing.T) (*Server, data.Task) {
	t.Helper()
	ctx := context.Background()
	db, err := localdb.Open(ctx, filepath.Join(t.TempDir(), "dory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	log, err := oplog.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: data.New(db, log)}
	task, err := s.Store.Tasks().Put(ctx, data.Task{Title: "Tarea"})
	if err != nil {
		t.Fatal(err)
	}
	return s, task
}

func TestPlanPending(t *testing.T) {
	ctx := context.Background()
	s, task := newPlanServer(t)
	var err error
	if pending, _ := s.planPending(ctx, task.ID); pending {
		t.Fatal("a task without a plan is pending")
	}
	if _, err = s.WritePlan(ctx, task.ID, "## Objetivo\n\nAlgo", -1, "agent"); err != nil {
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

func TestPlanDocument(t *testing.T) {
	ctx := context.Background()
	s, task := newPlanServer(t)
	p, err := s.PlanDocument(ctx, task.ID)
	if err != nil || p.Revision != 0 || p.BodyMD != "" {
		t.Fatalf("empty plan = %+v, %v", p, err)
	}
	if p, err = s.WritePlan(ctx, task.ID, "## Objetivo\n\nUno", 0, "agent"); err != nil || p.Revision != 1 || p.Version != 1 {
		t.Fatalf("first write = %+v, %v", p, err)
	}
	if _, err = s.WritePlan(ctx, task.ID, "otro", 0, "user"); !errors.Is(err, ErrPlanConflict) {
		t.Fatalf("stale write err = %v", err)
	}
	if p, err = s.EditPlan(ctx, task.ID, "Uno", "Dos", 1, "agent"); err != nil || p.BodyMD != "## Objetivo\n\nDos" || p.Revision != 2 {
		t.Fatalf("edit = %+v, %v", p, err)
	}
	if _, err = s.EditPlan(ctx, task.ID, "Tres", "x", -1, "agent"); !errors.Is(err, data.ErrInvalid) {
		t.Fatalf("missing text err = %v", err)
	}
	if _, _, err = s.acceptPlan(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if p, err = s.EditPlan(ctx, task.ID, "Dos", "Tres", -1, "user"); err != nil || p.AcceptedAt != nil || p.Version != 1 {
		t.Fatalf("editing an accepted plan = %+v, %v", p, err)
	}
}

func TestPlanFromSections(t *testing.T) {
	ctx := context.Background()
	s, task := newPlanServer(t)
	if _, err := s.Store.Plans().Put(ctx, data.Plan{TaskID: task.ID, Version: 1, Sections: []byte(`{"goal":"Meta","design":"Así"}`), OpenQuestions: []string{"¿Cuándo?"}}); err != nil {
		t.Fatal(err)
	}
	p, err := s.PlanDocument(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Objetivo\n\nMeta", "## Diseño\n\nAsí", "- ¿Cuándo?"} {
		if !strings.Contains(p.BodyMD, want) {
			t.Fatalf("legacy plan %q lacks %q", p.BodyMD, want)
		}
	}
}
