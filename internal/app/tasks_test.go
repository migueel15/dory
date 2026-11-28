package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/oplog"
)

type fakePulls struct {
	open   []github.Pull
	state  github.PullStatus
	cached map[string]github.PullStatus
	calls  int
}

func (f *fakePulls) Open(context.Context, bool) (*github.Overview, error) {
	return &github.Overview{Pulls: f.open}, nil
}

func (f *fakePulls) PullState(_ context.Context, _ string, number *int64, _ string) (github.PullStatus, error) {
	f.calls++
	s := f.state
	if s.Number == 0 && number != nil {
		s.Number = int(*number)
	}
	return s, nil
}

func (f *fakePulls) CachedPullState(repo string, number *int64, branch string) (github.PullStatus, bool) {
	if f.cached == nil {
		return github.PullStatus{}, false
	}
	key := repo + "#" + branch
	if number != nil {
		key = fmt.Sprintf("%s#%d", repo, *number)
	}
	s, ok := f.cached[key]
	return s, ok
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
	if _, _, err := a.RefreshPulls(ctx, src); err != nil {
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
	changed, more, err := a.RefreshPulls(ctx, src)
	if err != nil || !changed || more {
		t.Fatalf("changed=%v more=%v err=%v", changed, more, err)
	}
	changed, more, err = a.RefreshPulls(ctx, src)
	if err != nil || changed || more || src.calls != 1 {
		t.Fatalf("second refresh: changed=%v more=%v calls=%d err=%v", changed, more, src.calls, err)
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

func TestRecordPullStateMarksTaskDone(t *testing.T) {
	ctx := context.Background()
	a, repo := newTasks(t)
	if _, err := a.Store.Tasks().Create(ctx, data.Task{Title: "t", RepositoryID: &repo.ID, Branch: "feat"}); err != nil {
		t.Fatal(err)
	}
	src := &fakePulls{open: []github.Pull{{Repo: "o/r", Number: 7, HeadRef: "feat"}}}
	if _, _, err := a.RefreshPulls(ctx, src); err != nil {
		t.Fatal(err)
	}
	cards, err := a.Board(ctx, data.Filter{})
	if err != nil || cards[0].Column != "ready_to_merge" {
		t.Fatalf("before merge: cards=%+v err=%v", cards, err)
	}
	if err := a.RecordPullState(ctx, "o/r", 7, "draft"); err != nil {
		t.Fatal(err)
	}
	cards, err = a.Board(ctx, data.Filter{})
	if err != nil || cards[0].Column != "working" || cards[0].PRState != "draft" {
		t.Fatalf("after draft: cards=%+v err=%v", cards, err)
	}
	if err := a.RecordPullState(ctx, "o/r", 7, "merged"); err != nil {
		t.Fatal(err)
	}
	cards, err = a.Board(ctx, data.Filter{})
	if err != nil || cards[0].Column != "done" || cards[0].PRState != "merged" {
		t.Fatalf("after merge: cards=%+v err=%v", cards, err)
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

func TestRefreshPullsSeedsFromCachedPullState(t *testing.T) {
	ctx := context.Background()
	a, repo := newTasks(t)
	cached := map[string]github.PullStatus{}
	for i := 1; i <= 12; i++ {
		n := int64(i)
		if _, err := a.Store.Tasks().Create(ctx, data.Task{Title: fmt.Sprintf("t%d", i), RepositoryID: &repo.ID, PRNumber: &n}); err != nil {
			t.Fatal(err)
		}
		cached[fmt.Sprintf("o/r#%d", i)] = github.PullStatus{Number: i, State: "merged"}
	}
	src := &fakePulls{cached: cached}
	changed, more, err := a.RefreshPulls(ctx, src)
	if err != nil || !changed || more || src.calls != 0 {
		t.Fatalf("changed=%v more=%v calls=%d err=%v", changed, more, src.calls, err)
	}
	cards, err := a.Board(ctx, data.Filter{})
	if err != nil || len(cards) != 12 {
		t.Fatalf("cards=%d err=%v", len(cards), err)
	}
	for _, c := range cards {
		if c.Column != "done" {
			t.Fatalf("card %s column = %s", c.Title, c.Column)
		}
	}
}

func TestRefreshPullsReportsMoreUntilDrained(t *testing.T) {
	ctx := context.Background()
	a, repo := newTasks(t)
	for i := 1; i <= 10; i++ {
		n := int64(i)
		if _, err := a.Store.Tasks().Create(ctx, data.Task{Title: fmt.Sprintf("t%d", i), RepositoryID: &repo.ID, PRNumber: &n}); err != nil {
			t.Fatal(err)
		}
	}
	src := &fakePulls{state: github.PullStatus{State: "merged"}}
	changed, more, err := a.RefreshPulls(ctx, src)
	if err != nil || !changed || !more || src.calls != taskPullLookups {
		t.Fatalf("first batch: changed=%v more=%v calls=%d err=%v", changed, more, src.calls, err)
	}
	changed, more, err = a.RefreshPulls(ctx, src)
	if err != nil || !changed || more || src.calls != 10 {
		t.Fatalf("second batch: changed=%v more=%v calls=%d err=%v", changed, more, src.calls, err)
	}
	cards, err := a.Board(ctx, data.Filter{})
	if err != nil || len(cards) != 10 {
		t.Fatalf("cards=%d err=%v", len(cards), err)
	}
	for _, c := range cards {
		if c.Column != "done" {
			t.Fatalf("card %s column = %s", c.Title, c.Column)
		}
	}
}

func TestLaunchDelegatesToRemoteNodeWithoutLocalCheckout(t *testing.T) {
	ctx := context.Background()
	a, repo := newTasks(t)
	peer := &testPeer{}
	a.Plans = &Plans{Store: a.Store}
	a.Threads = &Threads{Store: a.Store, Node: a.Node, Peers: peer}
	task, err := a.Store.Tasks().Create(ctx, data.Task{
		Title:        "Remote task",
		ProjectID:    &repo.ProjectID,
		RepositoryID: &repo.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Repository is NOT cloned on this node, so local launch must fail.
	if _, _, err := a.Launch(ctx, task.ID, LaunchInput{Provider: "claude"}); !errors.Is(err, ErrRepositoryNotCloned) {
		t.Fatalf("local launch err = %v, want ErrRepositoryNotCloned", err)
	}
	// Remote launch on peer node must succeed without a local checkout.
	threadID, branch, err := a.Launch(ctx, task.ID, LaunchInput{Node: "peer-node", Provider: "claude"})
	if err != nil {
		t.Fatalf("remote launch err = %v", err)
	}
	if threadID != "remote-thread" || branch == "" {
		t.Fatalf("threadID=%q branch=%q", threadID, branch)
	}
	if peer.node != "peer-node" || peer.in.RepositoryID != repo.ID || peer.in.TaskID == nil || *peer.in.TaskID != task.ID || peer.in.Branch != branch {
		t.Fatalf("peer call = node:%q in:%+v", peer.node, peer.in)
	}
}
