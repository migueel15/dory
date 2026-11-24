package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/oplog"
)

func newPlanServer(t *testing.T) (*Plans, data.Task) {
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
	s := &Plans{Store: data.New(db, log)}
	task, err := s.Store.Tasks().Create(ctx, data.Task{Title: "Tarea"})
	if err != nil {
		t.Fatal(err)
	}
	return s, task
}

func TestPlanPending(t *testing.T) {
	ctx := context.Background()
	s, task := newPlanServer(t)
	var err error
	if pending, _ := s.Pending(ctx, task.ID); pending {
		t.Fatal("a task without a plan is pending")
	}
	if _, err = s.Write(ctx, task.ID, "## Objetivo\n\nAlgo", -1, "agent"); err != nil {
		t.Fatal(err)
	}
	if pending, _ := s.Pending(ctx, task.ID); !pending {
		t.Fatal("a proposed plan is not pending")
	}
	if _, _, err = s.Accept(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if pending, _ := s.Pending(ctx, task.ID); pending {
		t.Fatal("an accepted plan is still pending")
	}
}

func TestPlanDocument(t *testing.T) {
	ctx := context.Background()
	s, task := newPlanServer(t)
	p, err := s.Document(ctx, task.ID)
	if err != nil || p.Revision != 0 || p.BodyMD != "" {
		t.Fatalf("empty plan = %+v, %v", p, err)
	}
	if p, err = s.Write(ctx, task.ID, "## Objetivo\n\nUno", 0, "agent"); err != nil || p.Revision != 1 || p.Version != 1 {
		t.Fatalf("first write = %+v, %v", p, err)
	}
	if _, err = s.Write(ctx, task.ID, "otro", 0, "user"); !errors.Is(err, ErrPlanConflict) {
		t.Fatalf("stale write err = %v", err)
	}
	if p, err = s.Edit(ctx, task.ID, "Uno", "Dos", 1, "agent"); err != nil || p.BodyMD != "## Objetivo\n\nDos" || p.Revision != 2 {
		t.Fatalf("edit = %+v, %v", p, err)
	}
	if _, err = s.Edit(ctx, task.ID, "Tres", "x", -1, "agent"); !errors.Is(err, data.ErrInvalid) {
		t.Fatalf("missing text err = %v", err)
	}
	if _, _, err = s.Accept(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if p, err = s.Edit(ctx, task.ID, "Dos", "Tres", -1, "user"); err != nil || p.AcceptedAt != nil || p.Version != 1 {
		t.Fatalf("editing an accepted plan = %+v, %v", p, err)
	}
}

func TestPlanFromSections(t *testing.T) {
	ctx := context.Background()
	s, task := newPlanServer(t)
	if _, err := s.Store.Plans().Create(ctx, data.Plan{TaskID: task.ID, Version: 1, Sections: []byte(`{"goal":"Meta","design":"Así"}`), OpenQuestions: []string{"¿Cuándo?"}}); err != nil {
		t.Fatal(err)
	}
	p, err := s.Document(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Objetivo\n\nMeta", "## Diseño\n\nAsí", "- ¿Cuándo?"} {
		if !strings.Contains(p.BodyMD, want) {
			t.Fatalf("legacy plan %q lacks %q", p.BodyMD, want)
		}
	}
}

func TestWriteByAgent(t *testing.T) {
	ctx := context.Background()
	s, task := newPlanServer(t)
	if _, err := s.WriteByAgent(ctx, task.ID, "uno", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteByAgent(ctx, task.ID, "dos", nil); apperr.As(err) == nil || apperr.As(err).Code != apperr.Conflict {
		t.Fatalf("a blind rewrite must conflict, got %v", err)
	}
	rev := int64(1)
	if p, err := s.WriteByAgent(ctx, task.ID, "dos", &rev); err != nil || p.Revision != 2 {
		t.Fatalf("rewrite with the revision: %v %+v", err, p)
	}
	stale := int64(1)
	if _, err := s.EditByAgent(ctx, task.ID, "dos", "tres", &stale); !errors.Is(err, ErrPlanConflict) {
		t.Fatalf("edit on a stale revision: %v", err)
	}
}
