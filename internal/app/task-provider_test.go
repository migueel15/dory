package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/oplog"
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
		NodeID: "I_42",
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
	if err != nil || len(ghList) != 1 || ghList[0].Title != "GH Issue" || ghList[0].Adopted || ghList[0].ID != data.AdoptedTaskID("github", "I_42") {
		t.Fatalf("GitHubProvider.List failed: %+v %v", ghList, err)
	}
	if rows, _ := store.Tasks().ListAll(ctx, data.Filter{Fields: map[string]any{"source": "github"}}); len(rows) != 0 {
		t.Fatalf("a cached issue must not create tasks, got %+v", rows)
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

type stubIssueSource struct{ snaps []github.IssueSnapshot }

func (s *stubIssueSource) IssueSnapshots() []github.IssueSnapshot { return s.snaps }

func issue(n int, state string) github.Issue {
	return github.Issue{NodeID: fmt.Sprintf("I_%d", n), Repo: "org/demo", Number: n, Title: fmt.Sprintf("Issue %d", n), State: state}
}

func demoRepo(t *testing.T, store *data.Store) (data.Project, data.Repository) {
	t.Helper()
	proj, err := store.Projects().Create(t.Context(), data.Project{Slug: "demo", Name: "Demo"})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Repositories().Create(t.Context(), data.Repository{ProjectID: proj.ID, RemoteURL: "https://github.com/org/demo.git", DefaultBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	return proj, repo
}

func cardsByNumber(t *testing.T, a *Tasks, projectID string) map[int64]Card {
	t.Helper()
	cards, err := a.BoardMulti(t.Context(), TaskFilter{ProjectID: projectID, Source: "github"})
	if err != nil {
		t.Fatal(err)
	}
	out := map[int64]Card{}
	for _, c := range cards {
		if _, dup := out[*c.Number]; dup {
			t.Fatalf("issue #%d shows twice", *c.Number)
		}
		out[*c.Number] = c
	}
	return out
}

func TestRefreshExternalTasksOnlyCaches(t *testing.T) {
	ctx := t.Context()
	store := openAppStore(t)
	proj, _ := demoRepo(t, store)
	a := &Tasks{Store: store}

	src := &stubIssueSource{snaps: []github.IssueSnapshot{{Repo: "org/demo", Open: []github.Issue{issue(10, "open"), issue(11, "open")}, Closed: []github.Issue{issue(12, "closed")}, Complete: true}}}
	if _, err := a.RefreshExternalTasks(ctx, src); err != nil {
		t.Fatal(err)
	}
	cards := cardsByNumber(t, a, proj.ID)
	if len(cards) != 2 || cards[10].Adopted || cards[11].Adopted {
		t.Fatalf("expected issues 10 and 11 shown unadopted, got %+v", cards)
	}
	if rows, _ := store.Tasks().ListAll(ctx, data.Filter{}); len(rows) != 0 {
		t.Fatalf("the sync must not create tasks, got %d", len(rows))
	}

	// #11 is no longer open and the list is complete: it leaves the cache.
	src.snaps[0].Open = []github.Issue{issue(10, "open")}
	if _, err := a.RefreshExternalTasks(ctx, src); err != nil {
		t.Fatal(err)
	}
	if cards := cardsByNumber(t, a, proj.ID); len(cards) != 1 {
		t.Fatalf("expected only #10, got %+v", cards)
	}
	// An incomplete list keeps what it did not see.
	src.snaps[0] = github.IssueSnapshot{Repo: "org/demo", Complete: false}
	if _, err := a.RefreshExternalTasks(ctx, src); err != nil {
		t.Fatal(err)
	}
	if cards := cardsByNumber(t, a, proj.ID); len(cards) != 1 {
		t.Fatalf("expected #10 kept, got %+v", cards)
	}
}

func TestAdoptIsIdempotentAndFollowsTheIssue(t *testing.T) {
	ctx := t.Context()
	store := openAppStore(t)
	proj, _ := demoRepo(t, store)
	a := &Tasks{Store: store}
	src := &stubIssueSource{snaps: []github.IssueSnapshot{{Repo: "org/demo", Open: []github.Issue{issue(20, "open")}, Complete: true}}}
	if _, err := a.RefreshExternalTasks(ctx, src); err != nil {
		t.Fatal(err)
	}
	id := data.AdoptedTaskID("github", "I_20")

	// The first write adopts: here, moving it to Ready.
	if _, err := a.Patch(ctx, id, nil, data.Fields{"stage": "ready"}); err != nil {
		t.Fatal(err)
	}
	again, err := a.Adopt(ctx, id)
	if err != nil || again.ID != id || again.Stage != "ready" || again.Title != "Issue 20" {
		t.Fatalf("unexpected adopted task: %+v (%v)", again, err)
	}
	cards := cardsByNumber(t, a, proj.ID)
	if c := cards[20]; !c.Adopted || c.ID != id || c.Column != "ready" {
		t.Fatalf("expected one adopted card in Ready, got %+v", c)
	}

	// Closed on GitHub: the adopted task keeps its cache row and moves to Done.
	src.snaps[0] = github.IssueSnapshot{Repo: "org/demo", Closed: []github.Issue{issue(20, "closed")}, Complete: true}
	if _, err := a.RefreshExternalTasks(ctx, src); err != nil {
		t.Fatal(err)
	}
	if c := cardsByNumber(t, a, proj.ID)[20]; c.Column != "done" {
		t.Fatalf("expected the closed issue in Done, got %q", c.Column)
	}
}

func TestDetectIssueLinksAttachesToInternalTask(t *testing.T) {
	ctx := t.Context()
	store := openAppStore(t)
	proj, repo := demoRepo(t, store)
	a := &Tasks{Store: store}
	task, err := store.Tasks().Create(ctx, data.Task{ProjectID: &proj.ID, RepositoryID: &repo.ID, Title: "Dory task"})
	if err != nil {
		t.Fatal(err)
	}
	th, err := store.Threads().Create(ctx, data.Thread{NodeID: "n1", ProjectID: &proj.ID, TaskID: &task.ID, Provider: "claude", Branch: "feat/30-x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ThreadPulls().Link(ctx, th.ID, repo.ID, 31, th.Branch, "mcp"); err != nil {
		t.Fatal(err)
	}
	iss := issue(30, "open")
	iss.Pulls = []github.Ref{{Number: 31}}
	if _, err := a.RefreshExternalTasks(ctx, &stubIssueSource{snaps: []github.IssueSnapshot{{Repo: "org/demo", Open: []github.Issue{iss}, Complete: true}}}); err != nil {
		t.Fatal(err)
	}
	if err := a.DetectIssueLinks(ctx, "n1"); err != nil {
		t.Fatal(err)
	}
	cards := cardsByNumber(t, a, proj.ID)
	if c := cards[30]; c.ID != task.ID || !c.Adopted || c.Title != "Issue 30" {
		t.Fatalf("expected the issue shown as the Dory task, got %+v", c)
	}
}

func TestArchiveImportedTasks(t *testing.T) {
	ctx := t.Context()
	store := openAppStore(t)
	proj, repo := demoRepo(t, store)
	number := int64(5)
	old, err := store.Tasks().Create(ctx, data.Task{Source: "github", ProjectID: &proj.ID, RepositoryID: &repo.ID, Number: &number})
	if err != nil {
		t.Fatal(err)
	}
	internal, err := store.Tasks().Create(ctx, data.Task{ProjectID: &proj.ID, Title: "keep"})
	if err != nil {
		t.Fatal(err)
	}
	if err := (&Tasks{Store: store}).ArchiveImportedTasks(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Tasks().Get(ctx, old.ID); !errors.Is(err, data.ErrNotFound) {
		t.Fatalf("expected the imported task archived, got %v", err)
	}
	if _, err := store.Tasks().Get(ctx, internal.ID); err != nil {
		t.Fatalf("internal task must stay: %v", err)
	}
}
