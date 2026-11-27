package app

import (
	"testing"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/github"
)

func TestInternalAndGitHubProvidersAndBranchDetection(t *testing.T) {
	ctx := t.Context()
	store := openAppStore(t)

	proj, err := store.Projects().Create(ctx, data.Project{Slug: "demo", Name: "Demo"})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Repositories().Create(ctx, data.Repository{ProjectID: proj.ID, RemoteURL: "https://github.com/org/demo.git", DefaultBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}

	// InternalProvider
	ip := &InternalProvider{Store: store}
	created, err := ip.Create(ctx, TaskCreateInput{ProjectID: proj.ID, Title: "Internal 1", Description: "D"})
	if err != nil || created.Source != "internal" || created.Title != "Internal 1" {
		t.Fatalf("InternalProvider.Create failed: %+v %v", created, err)
	}
	list, err := ip.List(ctx, TaskFilter{ProjectID: proj.ID})
	if err != nil || len(list) != 1 {
		t.Fatalf("InternalProvider.List failed: %+v %v", list, err)
	}

	// GitHubProvider from cache
	err = UpsertExternalTaskCache(ctx, store.LocalDB(), repo.ID, "org/demo", github.Issue{
		Number: 42,
		Title:  "GH Issue",
		State:  "open",
		Meta:   github.Meta{Assignees: []string{"alice"}},
	}, "Issue body")
	if err != nil {
		t.Fatal(err)
	}
	gp := &GitHubProvider{Store: store}
	ghList, err := gp.List(ctx, TaskFilter{ProjectID: proj.ID, Scope: "mine", Viewer: "alice"})
	if err != nil || len(ghList) != 1 || ghList[0].Title != "GH Issue" {
		t.Fatalf("GitHubProvider.List failed: %+v %v", ghList, err)
	}
	ghListOther, err := gp.List(ctx, TaskFilter{ProjectID: proj.ID, Scope: "mine", Viewer: "bob"})
	if err != nil || len(ghListOther) != 0 {
		t.Fatalf("expected 0 issues for bob, got %d", len(ghListOther))
	}

	// Branch detection + respecting manual unlink
	th, err := store.Threads().Create(ctx, data.Thread{NodeID: "n1", ProjectID: &proj.ID, TaskID: &created.ID, Provider: "claude", Branch: "feat/auto-pr"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.LocalDB().ExecContext(ctx, `
		INSERT INTO task_pulls(repo, ref, number, state, head_ref, updated_at)
		VALUES ('org/demo', '#99', 99, 'open', 'feat/auto-pr', '2026-01-01T00:00:00Z')
	`)
	if err != nil {
		t.Fatal(err)
	}
	tasksApp := &Tasks{Store: store}
	if err := tasksApp.DetectThreadPullsByBranch(ctx, "n1"); err != nil {
		t.Fatal(err)
	}
	tps, err := store.ThreadPulls().ListByThread(ctx, th.ID)
	if err != nil || len(tps) != 1 || tps[0].Number != 99 || tps[0].LinkedBy != "branch" {
		t.Fatalf("expected auto-linked PR #99, got %+v (%v)", tps, err)
	}

	// Manual unlink should not be recreated
	if err := store.ThreadPulls().Unlink(ctx, th.ID, repo.ID, 99); err != nil {
		t.Fatal(err)
	}
	if err := tasksApp.DetectThreadPullsByBranch(ctx, "n1"); err != nil {
		t.Fatal(err)
	}
	tps, err = store.ThreadPulls().ListByThread(ctx, th.ID)
	if err != nil || len(tps) != 0 {
		t.Fatalf("expected unlinked PR #99 to stay unlinked, got %+v", tps)
	}
}
