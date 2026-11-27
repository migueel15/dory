package data

import (
	"errors"
	"testing"
)

func TestAdoptedTaskIDDeterministic(t *testing.T) {
	if AdoptedTaskID("github", "I_1") != AdoptedTaskID("github", "I_1") {
		t.Fatal("expected a deterministic id")
	}
	if AdoptedTaskID("github", "I_1") == AdoptedTaskID("github", "I_2") || AdoptedTaskID("github", "I_1") == AdoptedTaskID("linear", "I_1") {
		t.Fatal("expected distinct ids for distinct keys")
	}
}

func TestAdoptTaskAndMoveStage(t *testing.T) {
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
	ref := ExternalRef{Source: "github", ExternalID: "I_42", ProjectID: &proj.ID, RepositoryID: repo.ID, Number: 42, Title: "Issue 42"}

	task1, err := store.AdoptTask(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if task1.ID != AdoptedTaskID("github", "I_42") || task1.Source != "github" || task1.ExternalID != "I_42" || task1.Number == nil || *task1.Number != 42 || task1.Title != "Issue 42" {
		t.Fatalf("unexpected adopted task: %+v", task1)
	}
	// Idempotent
	task2, err := store.AdoptTask(ctx, ref)
	if err != nil || task1.ID != task2.ID {
		t.Fatalf("expected the same task, got %+v (%v)", task2, err)
	}
	// An archived adopted task is restored with the same id.
	if err := store.Tasks().Archive(ctx, task1.ID); err != nil {
		t.Fatal(err)
	}
	if task3, err := store.AdoptTask(ctx, ref); err != nil || task3.ID != task1.ID || task3.ArchivedAt != nil {
		t.Fatalf("expected the archived task restored, got %+v (%v)", task3, err)
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

func TestAttachExternalKeepsTaskID(t *testing.T) {
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
	internal, err := store.Tasks().Create(ctx, Task{ProjectID: &proj.ID, Title: "Mine", Stage: "ready"})
	if err != nil {
		t.Fatal(err)
	}
	ref := ExternalRef{Source: "github", ExternalID: "I_7", RepositoryID: repo.ID, Number: 7}
	got, err := store.AttachExternal(ctx, internal.ID, ref)
	if err != nil || got.ID != internal.ID || got.Source != "github" || got.ExternalID != "I_7" || got.Stage != "ready" {
		t.Fatalf("unexpected attached task: %+v (%v)", got, err)
	}
	// Adopting the same item now returns the task that holds it.
	if adopted, err := store.AdoptTask(ctx, ref); err != nil || adopted.ID != internal.ID {
		t.Fatalf("expected the holder task, got %+v (%v)", adopted, err)
	}
	other, err := store.Tasks().Create(ctx, Task{ProjectID: &proj.ID, Title: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AttachExternal(ctx, other.ID, ref); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected a conflict, got %v", err)
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
