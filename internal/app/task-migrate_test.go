package app

import (
	"path/filepath"
	"testing"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/oplog"
)

func openAppStore(t *testing.T) *data.Store {
	t.Helper()
	db, err := localdb.Open(t.Context(), filepath.Join(t.TempDir(), "dory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	log, err := oplog.Open(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	return data.New(db, log)
}

func TestMigrateLegacyTasks(t *testing.T) {
	ctx := t.Context()
	store := openAppStore(t)
	tasksApp := &Tasks{Store: store}

	proj, err := store.Projects().Create(ctx, data.Project{Slug: "demo", Name: "Demo"})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Repositories().Create(ctx, data.Repository{ProjectID: proj.ID, RemoteURL: "https://github.com/org/demo.git"})
	if err != nil {
		t.Fatal(err)
	}

	issueNum := int64(42)
	prNum := int64(101)
	legacyIssue, err := store.Tasks().Create(ctx, data.Task{
		ProjectID:    &proj.ID,
		RepositoryID: &repo.ID,
		Title:        "Old issue task",
		Stage:        "ready",
		IssueNumber:  &issueNum,
		PRNumber:     &prNum,
		Branch:       "feat/42-old",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherTask, err := store.Tasks().Create(ctx, data.Task{
		ProjectID: &proj.ID,
		Title:     "Blocker",
	})
	if err != nil {
		t.Fatal(err)
	}
	th1, err := store.Threads().Create(ctx, data.Thread{NodeID: "n1", ProjectID: &proj.ID, TaskID: &legacyIssue.ID, Provider: "claude", Branch: "feat/42-old"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.Plans().Create(ctx, data.Plan{TaskID: legacyIssue.ID, Version: 1, BodyMD: "# Plan"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Dependencies().Upsert(ctx, data.Dependency{TaskID: legacyIssue.ID, DependsOnID: otherTask.ID}); err != nil {
		t.Fatal(err)
	}

	// Internal task with PR and no threads (PR discarded)
	prNum2 := int64(202)
	internalNoThreads, err := store.Tasks().Create(ctx, data.Task{
		ProjectID:    &proj.ID,
		RepositoryID: &repo.ID,
		Title:        "Internal no threads",
		PRNumber:     &prNum2,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := tasksApp.MigrateLegacyTasks(ctx); err != nil {
		t.Fatal(err)
	}

	// Idempotent second run
	if err := tasksApp.MigrateLegacyTasks(ctx); err != nil {
		t.Fatal(err)
	}

	newID := data.ExternalTaskID(repo.ID, 42)
	migrated, err := store.Tasks().Get(ctx, newID)
	if err != nil {
		t.Fatalf("expected migrated task %s: %v", newID, err)
	}
	if migrated.Source != "github" || migrated.Number == nil || *migrated.Number != 42 || migrated.Stage != "ready" {
		t.Fatalf("unexpected migrated task: %+v", migrated)
	}

	// Old task archived
	if _, err := store.Tasks().Get(ctx, legacyIssue.ID); err == nil {
		t.Fatalf("expected old legacy task to be archived")
	}

	// Thread moved & thread_pull created
	gotTh, err := store.Threads().Get(ctx, th1.ID)
	if err != nil || gotTh.TaskID == nil || *gotTh.TaskID != newID {
		t.Fatalf("expected thread moved to %s, got %+v (%v)", newID, gotTh, err)
	}
	tps, err := store.ThreadPulls().ListByThread(ctx, th1.ID)
	if err != nil || len(tps) != 1 || tps[0].Number != 101 {
		t.Fatalf("expected 1 thread_pull #101, got %+v (%v)", tps, err)
	}

	// Plan moved
	gotPlan, err := store.Plans().Get(ctx, plan.ID)
	if err != nil || gotPlan.TaskID != newID {
		t.Fatalf("expected plan moved to %s, got %+v (%v)", newID, gotPlan, err)
	}

	// Dependency moved
	deps, err := store.Dependencies().ListAll(ctx, data.Filter{Fields: map[string]any{"task_id": newID}})
	if err != nil || len(deps) != 1 || deps[0].DependsOnID != otherTask.ID {
		t.Fatalf("expected dependency moved, got %+v (%v)", deps, err)
	}

	// Internal task without threads had PR cleared
	gotInternal, err := store.Tasks().Get(ctx, internalNoThreads.ID)
	if err != nil || gotInternal.PRNumber != nil {
		t.Fatalf("expected internal task pr_number cleared, got %+v (%v)", gotInternal, err)
	}
}
