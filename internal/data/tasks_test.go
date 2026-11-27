package data

import (
	"testing"
)

func TestExternalTaskIDDeterministic(t *testing.T) {
	id1 := ExternalTaskID("repo-1", 42)
	id2 := ExternalTaskID("repo-1", 42)
	id3 := ExternalTaskID("repo-1", 43)
	if id1 != id2 {
		t.Fatalf("expected deterministic id, got %q != %q", id1, id2)
	}
	if id1 == id3 {
		t.Fatalf("expected distinct ids for different numbers")
	}
}

func TestEnsureExternalTaskAndMoveStage(t *testing.T) {
	store := open(t)
	ctx := t.Context()

	proj, err := store.Projects().Create(ctx, Project{Slug: "demo", Name: "Demo"})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Repositories().Create(ctx, Repository{ProjectID: proj.ID, RemoteURL: "https://github.com/org/demo.git"})
	if err != nil {
		t.Fatal(err)
	}

	task1, err := store.EnsureExternalTask(ctx, &proj.ID, repo.ID, 42)
	if err != nil {
		t.Fatal(err)
	}
	if task1.ID != ExternalTaskID(repo.ID, 42) || task1.Source != "github" || task1.Number == nil || *task1.Number != 42 {
		t.Fatalf("unexpected external task: %+v", task1)
	}
	// Idempotent
	task2, err := store.EnsureExternalTask(ctx, &proj.ID, repo.ID, 42)
	if err != nil {
		t.Fatal(err)
	}
	if task1.ID != task2.ID {
		t.Fatalf("expected same task id")
	}

	// AcceptPlan does not change stage
	plan, err := store.Plans().Create(ctx, Plan{TaskID: task1.ID, Version: 1, BodyMD: "# Plan"})
	if err != nil {
		t.Fatal(err)
	}
	updatedTask, acceptedPlan, err := store.AcceptPlan(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if acceptedPlan.AcceptedAt == nil {
		t.Fatalf("expected accepted_at to be set")
	}
	if updatedTask.Stage != "" {
		t.Fatalf("AcceptPlan should not modify stage, got %q", updatedTask.Stage)
	}

	// MoveStage to inbox clears accepted_at
	moved, err := store.MoveStage(ctx, task1.ID, "inbox")
	if err != nil {
		t.Fatal(err)
	}
	if moved.Stage != "inbox" {
		t.Fatalf("expected stage inbox, got %q", moved.Stage)
	}
	reloadedPlan, err := store.Plans().Get(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloadedPlan.AcceptedAt != nil {
		t.Fatalf("expected accepted_at cleared when moving to inbox")
	}
}

func TestThreadPullsLinkUnlink(t *testing.T) {
	store := open(t)
	ctx := t.Context()

	proj, err := store.Projects().Create(ctx, Project{Slug: "demo", Name: "Demo"})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Repositories().Create(ctx, Repository{ProjectID: proj.ID, RemoteURL: "https://github.com/org/demo.git"})
	if err != nil {
		t.Fatal(err)
	}
	th, err := store.Threads().Create(ctx, Thread{NodeID: "n1", ProjectID: &proj.ID, Provider: "claude", Title: "T1"})
	if err != nil {
		t.Fatal(err)
	}

	tp, err := store.ThreadPulls().Link(ctx, th.ID, repo.ID, 12, "feat/x", "mcp")
	if err != nil {
		t.Fatal(err)
	}
	if tp.Number != 12 || tp.LinkedBy != "mcp" {
		t.Fatalf("unexpected thread pull: %+v", tp)
	}

	list, err := store.ThreadPulls().ListByThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 thread pull, got %d", len(list))
	}

	if err := store.ThreadPulls().Unlink(ctx, th.ID, repo.ID, 12); err != nil {
		t.Fatal(err)
	}
	list, err = store.ThreadPulls().ListByThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 active thread pulls after unlink, got %d", len(list))
	}
}
