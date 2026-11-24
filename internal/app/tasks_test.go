package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/oplog"
)

type fakePulls struct {
	open  []github.Pull
	state github.PullStatus
	calls int
}

func (f *fakePulls) Open(context.Context, bool) (*github.Overview, error) {
	return &github.Overview{Pulls: f.open}, nil
}

func (f *fakePulls) PullState(context.Context, string, *int64, string) (github.PullStatus, error) {
	f.calls++
	return f.state, nil
}

func newTasks(t *testing.T) (*Tasks, data.Repository) {
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
	store := data.New(db, log)
	project, err := store.Projects().Create(ctx, data.Project{Name: "p"})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Repositories().Create(ctx, data.Repository{ProjectID: project.ID, RemoteURL: "https://github.com/o/r.git"})
	if err != nil {
		t.Fatal(err)
	}
	return &Tasks{Store: store, Node: log.Node()}, repo
}

func TestBoardUsesOnlyTheLocalCache(t *testing.T) {
	ctx := context.Background()
	a, repo := newTasks(t)
	task, err := a.Store.Tasks().Create(ctx, data.Task{Title: "t", RepositoryID: &repo.ID, Branch: "feat"})
	if err != nil {
		t.Fatal(err)
	}
	src := &fakePulls{open: []github.Pull{{Repo: "o/r", Number: 4, HeadRef: "feat", Draft: true}}}
	if _, err := a.RefreshPulls(ctx, src); err != nil {
		t.Fatal(err)
	}
	cards, err := a.Board(ctx, data.Filter{})
	if err != nil || len(cards) != 1 {
		t.Fatalf("cards = %+v: %v", cards, err)
	}
	if c := cards[0]; c.ID != task.ID || c.Column != "working" || c.PRState != "draft" || c.PRNumber == nil || *c.PRNumber != 4 {
		t.Fatalf("card = %+v", c)
	}
	if src.calls != 0 {
		t.Fatalf("PullState calls = %d", src.calls)
	}
}

func TestRefreshPullsCachesFinalStateOnce(t *testing.T) {
	ctx := context.Background()
	a, repo := newTasks(t)
	if _, err := a.Store.Tasks().Create(ctx, data.Task{Title: "t", RepositoryID: &repo.ID, Branch: "feat"}); err != nil {
		t.Fatal(err)
	}
	src := &fakePulls{state: github.PullStatus{Number: 9, State: "merged"}}
	changed, err := a.RefreshPulls(ctx, src)
	if err != nil || !changed {
		t.Fatalf("changed = %v: %v", changed, err)
	}
	changed, err = a.RefreshPulls(ctx, src)
	if err != nil || changed || src.calls != 1 {
		t.Fatalf("second refresh: changed=%v calls=%d err=%v", changed, src.calls, err)
	}
	cards, _ := a.Board(ctx, data.Filter{})
	if cards[0].Column != "done" {
		t.Fatalf("column = %s", cards[0].Column)
	}
}

func TestCheckDoneRefusesUnmergedPull(t *testing.T) {
	ctx := context.Background()
	a, _ := newTasks(t)
	n := int64(3)
	if err := a.CheckDone(ctx, data.Task{PRNumber: &n}); err == nil {
		t.Fatal("expected refusal")
	}
	if err := a.CheckDone(ctx, data.Task{}); err != nil {
		t.Fatal(err)
	}
}

func TestFinishIsIdempotentWithoutThreads(t *testing.T) {
	ctx := context.Background()
	a, _ := newTasks(t)
	a.ArchiveOnFinish = func() bool { return true }
	if _, err := a.Store.Tasks().Create(ctx, data.Task{Title: "t", Stage: "cancelled"}); err != nil {
		t.Fatal(err)
	}
	a.Finish(ctx)
	a.Finish(ctx)
}
