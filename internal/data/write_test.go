package data

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/migueel15/dory/internal/oplog"
)

func TestConcurrentPatches(t *testing.T) {
	ctx := t.Context()
	s := open(t)
	task, err := s.Tasks().Create(ctx, Task{Title: "Antes", Description: "Antes"})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	done := make(chan error, 2)
	for _, fields := range []Fields{{"title": "Título"}, {"description": "Descripción"}} {
		go func() {
			<-start
			_, err := s.Tasks().Patch(ctx, task.ID, fields)
			done <- err
		}()
	}
	close(start)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Tasks().Get(ctx, task.ID)
	if err != nil || got.Title != "Título" || got.Description != "Descripción" {
		t.Fatalf("task = %+v, err = %v", got, err)
	}
}

func TestCreateAndPatchVersions(t *testing.T) {
	ctx := t.Context()
	s := open(t)
	stamp := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return stamp }
	issue := int64(9007199254740993)
	task, err := s.Tasks().Create(ctx, Task{ID: "task", IssueNumber: &issue})
	if err != nil || task.Stage != "inbox" || task.IssueNumber == nil || *task.IssueNumber != issue {
		t.Fatalf("task = %+v, err = %v", task, err)
	}
	if _, err := s.Tasks().Create(ctx, Task{ID: task.ID}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate = %v", err)
	}
	out, err := s.Tasks().PatchIf(ctx, task.ID, task.UpdatedAt, Fields{"issue_number": nil, "title": "Editada"})
	if err != nil || out.IssueNumber != nil || out.UpdatedAt == task.UpdatedAt {
		t.Fatalf("patched task = %+v, err = %v", out, err)
	}
	if _, err := s.Tasks().PatchIf(ctx, task.ID, task.UpdatedAt, Fields{"title": "Antigua"}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale patch = %v", err)
	}
	for _, invalid := range []Fields{{"abracadabra": true}, {"id": "other"}, {"title": nil}, {"issue_number": 1.5}, {"updated_at": task.UpdatedAt}} {
		if _, err := s.Tasks().Patch(ctx, task.ID, invalid); !errors.Is(err, ErrInvalid) {
			t.Fatalf("patch %v = %v", invalid, err)
		}
	}
	project, err := s.Projects().Create(ctx, Project{Slug: "proyecto"})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := s.Repositories().Create(ctx, Repository{ProjectID: project.ID, RemoteURL: "https://github.com/org/proyecto"})
	if err != nil || repo.DefaultBranch != "main" {
		t.Fatalf("repo = %+v, err = %v", repo, err)
	}
	repo, err = s.Repositories().Patch(ctx, repo.ID, Fields{"default_branch": ""})
	if err != nil || repo.DefaultBranch != "" {
		t.Fatalf("patched repo = %+v, err = %v", repo, err)
	}
}

func TestEnsureThreadTaskRollback(t *testing.T) {
	ctx := t.Context()
	s := open(t)
	thread, err := s.Threads().Create(ctx, Thread{NodeID: s.log.Node(), Provider: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	before := opCount(t, s)
	if _, err := s.db.ExecContext(ctx, `CREATE TRIGGER refuse_link BEFORE UPDATE OF task_id ON threads
		BEGIN SELECT RAISE(ABORT, 'refuse link'); END`); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s.log.OnChange = func(string, string) { calls++ }
	if _, err := s.EnsureThreadTask(ctx, thread.ID, "Tarea", ""); err == nil {
		t.Fatal("link should fail")
	}
	if got := opCount(t, s); got != before || calls != 0 {
		t.Fatalf("ops = %d, notifications = %d", got, calls)
	}
	if _, err := s.Tasks().Get(ctx, threadTaskID(thread.ID)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rolled back task: %v", err)
	}
}

func TestTransactionNotifiesAfterCommit(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	s := open(t)
	var events []string
	s.log.OnChange = func(entity, id string) {
		events = append(events, id)
		for _, taskID := range []string{"first", "second"} {
			if _, err := s.Tasks().Get(ctx, taskID); err != nil {
				t.Errorf("notification for %s/%s: %s is not committed: %v", entity, id, taskID, err)
			}
		}
	}
	err := s.log.Tx(ctx, func(tx *oplog.Tx) error {
		for _, id := range []string{"first", "second"} {
			if _, err := tx.Write("task", id, Fields{"title": id, "created_at": "x", "updated_at": "x"}); err != nil {
				return err
			}
		}
		if len(events) != 0 {
			t.Errorf("notifications before commit: %v", events)
		}
		return nil
	})
	if err != nil || len(events) != 2 || events[0] != "first" || events[1] != "second" {
		t.Fatalf("events = %v, err = %v", events, err)
	}
}
