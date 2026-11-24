package data

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/oplog"
)

func open(t *testing.T) *Store {
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
	return New(db, log)
}

func opCount(t *testing.T, s *Store) int {
	var n int
	s.db.QueryRow(`SELECT count(*) FROM ops`).Scan(&n)
	return n
}

func TestEnsureThreadTask(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	p, err := s.Projects().Create(ctx, Project{Slug: "plan", Name: "Plan"})
	if err != nil {
		t.Fatal(err)
	}
	th, err := s.Threads().Create(ctx, Thread{NodeID: s.log.Node(), ProjectID: &p.ID, Provider: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.EnsureThreadTask(ctx, th.ID, "Planificar", "Contexto")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.EnsureThreadTask(ctx, th.ID, "Otro título", "")
	if err != nil || second.ID != first.ID || second.Title != "Planificar" || first.ProjectID == nil || *first.ProjectID != p.ID {
		t.Fatalf("first=%+v second=%+v err=%v", first, second, err)
	}
	got, err := s.Threads().Get(ctx, th.ID)
	if err != nil || got.TaskID == nil || *got.TaskID != first.ID {
		t.Fatalf("thread=%+v err=%v", got, err)
	}
	tasks, err := s.Tasks().List(ctx, Filter{})
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks=%+v err=%v", tasks, err)
	}
	other, err := s.Tasks().Create(ctx, Task{Title: "Existente"})
	if err != nil {
		t.Fatal(err)
	}
	th2, err := s.Threads().Create(ctx, Thread{NodeID: s.log.Node(), TaskID: &other.ID, Provider: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := s.EnsureThreadTask(ctx, th2.ID, "", "")
	if err != nil || linked.ID != other.ID {
		t.Fatalf("linked=%+v err=%v", linked, err)
	}
}

func TestTaskLifecycle(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	p, err := s.Projects().Create(ctx, Project{Slug: "dory", Name: "Dory"})
	if err != nil {
		t.Fatal(err)
	}
	issue := int64(148)
	task, err := s.Tasks().Create(ctx, Task{ProjectID: &p.ID, Title: "Base local", IssueNumber: &issue})
	if err != nil {
		t.Fatal(err)
	}
	if task.ID == "" || task.Stage != "inbox" || task.CreatedAt == "" || *task.IssueNumber != 148 {
		t.Fatalf("defaults not applied: %+v", task)
	}

	before := opCount(t, s)
	if _, err = s.Tasks().Patch(ctx, task.ID, Fields{"stage": task.Stage}); err != nil {
		t.Fatal(err)
	}
	if opCount(t, s) != before {
		t.Fatal("unchanged patch wrote an op")
	}
	task.Stage = "ready"
	if task, err = s.Tasks().Patch(ctx, task.ID, Fields{"stage": task.Stage}); err != nil || task.Stage != "ready" {
		t.Fatalf("update: %+v %v", task, err)
	}
	var fields string
	s.db.QueryRow(`SELECT fields FROM ops ORDER BY hlc DESC LIMIT 1`).Scan(&fields)
	if fields != `{"stage":"ready","updated_at":"`+task.UpdatedAt+`"}` {
		t.Fatalf("op carries more than the change: %s", fields)
	}

	list, err := s.Tasks().List(ctx, Filter{Fields: map[string]any{"project_id": p.ID}})
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", list, err)
	}
	if err = s.Tasks().Archive(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Tasks().Get(ctx, task.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("archived task still visible: %v", err)
	}
	if _, err = s.Tasks().List(ctx, Filter{Fields: map[string]any{"title; --": 1}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("unknown filter field accepted")
	}
}

func TestRepositoryIDFromRemote(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	p, _ := s.Projects().Create(ctx, Project{Slug: "dory", Name: "Dory"})
	r, err := s.Repositories().Upsert(ctx, Repository{ProjectID: p.ID, RemoteURL: "git@github.com:migueel15/dory.git"})
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != oplog.RepositoryID("https://github.com/migueel15/dory") || r.DefaultBranch != "main" {
		t.Fatalf("got %+v", r)
	}
}

func TestPlansAndMemoriesKeepJSON(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	task, _ := s.Tasks().Create(ctx, Task{Title: "Con plan"})
	plan, err := s.Plans().Create(ctx, Plan{TaskID: task.ID, Version: 1, Sections: []byte(`{"objetivo":"SQLite"}`), OpenQuestions: []string{"¿FTS?"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(plan.Sections) != `{"objetivo":"SQLite"}` || plan.OpenQuestions[0] != "¿FTS?" || plan.Author != "agent" {
		t.Fatalf("got %+v", plan)
	}
	m, err := s.Memories().Create(ctx, Memory{Scope: "global", Title: "Decisión", Content: "La v2 usa SQLite", Labels: []string{"v2"}})
	if err != nil || m.Kind != "context" || m.Labels[0] != "v2" {
		t.Fatalf("memory: %+v %v", m, err)
	}
	if _, err = s.Memories().Create(ctx, Memory{Scope: "global", ScopeID: "x", Content: "mal"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("check constraint not enforced: %v", err)
	}

	hits, err := s.Search(ctx, "sqli", 10)
	if err != nil || len(hits) != 1 || hits[0].Entity != "memory" || hits[0].ID != m.ID {
		t.Fatalf("search: %+v %v", hits, err)
	}
	if _, err = s.Search(ctx, `"OR NEAR(`, 10); err != nil {
		t.Fatalf("operators not escaped: %v", err)
	}
}

func TestBlockers(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	t2, _ := s.Tasks().Create(ctx, Task{Title: "Dos"})
	t3, _ := s.Tasks().Create(ctx, Task{Title: "Tres"})
	if _, err := s.Dependencies().Upsert(ctx, Dependency{TaskID: t3.ID, DependsOnID: t2.ID}); err != nil {
		t.Fatal(err)
	}
	blockers, err := s.Blockers(ctx, t3.ID)
	if err != nil || len(blockers) != 1 || blockers[0].ID != t2.ID {
		t.Fatalf("blockers: %+v %v", blockers, err)
	}
	t2.Stage = "cancelled"
	s.Tasks().Patch(ctx, t2.ID, Fields{"stage": t2.Stage})
	if blockers, _ = s.Blockers(ctx, t3.ID); len(blockers) != 0 {
		t.Fatalf("cancelled task still blocks: %+v", blockers)
	}
}

func TestRepositoryLocationUpsertGet(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	node := s.log.Node()
	if _, err := s.RepositoryLocations().Upsert(ctx, RepositoryLocation{RepositoryID: "r1", NodeID: node, Path: "/src/r1"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.RepositoryLocations().Get(ctx, "r1/"+node)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/src/r1" {
		t.Fatalf("path = %q", got.Path)
	}
}

func TestLinkThread(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	p, _ := s.Projects().Create(ctx, Project{Slug: "a", Name: "A"})
	q, _ := s.Projects().Create(ctx, Project{Slug: "b", Name: "B"})
	t1, _ := s.Tasks().Create(ctx, Task{Title: "Uno", ProjectID: &p.ID})
	t2, _ := s.Tasks().Create(ctx, Task{Title: "Dos", ProjectID: &p.ID})
	foreign, _ := s.Tasks().Create(ctx, Task{Title: "Ajena", ProjectID: &q.ID})
	th, _ := s.Threads().Create(ctx, Thread{NodeID: "otro-nodo", ProjectID: &p.ID, Provider: "codex"})

	got, err := s.LinkThread(ctx, th.ID, t1.ID, false)
	if err != nil || got.TaskID == nil || *got.TaskID != t1.ID {
		t.Fatalf("link: %+v %v", got, err)
	}
	n := opCount(t, s)
	if _, err = s.LinkThread(ctx, th.ID, t1.ID, false); err != nil || opCount(t, s) != n {
		t.Fatalf("relink wrote ops or failed: %v", err)
	}
	if _, err = s.LinkThread(ctx, th.ID, t2.ID, false); !errors.Is(err, ErrInvalid) {
		t.Fatalf("move without replace: %v", err)
	}
	if got, err = s.LinkThread(ctx, th.ID, t2.ID, true); err != nil || *got.TaskID != t2.ID {
		t.Fatalf("replace: %+v %v", got, err)
	}
	if _, err = s.LinkThread(ctx, th.ID, foreign.ID, true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("foreign project: %v", err)
	}
	s.Tasks().Archive(ctx, t1.ID)
	if _, err = s.LinkThread(ctx, th.ID, t1.ID, true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("archived task: %v", err)
	}
	if got, err = s.UnlinkThread(ctx, th.ID); err != nil || got.TaskID != nil {
		t.Fatalf("unlink: %+v %v", got, err)
	}
}

func TestDependencies(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	a, _ := s.Tasks().Create(ctx, Task{Title: "A"})
	b, _ := s.Tasks().Create(ctx, Task{Title: "B"})
	c, _ := s.Tasks().Create(ctx, Task{Title: "C"})
	if _, err := s.AddDependency(ctx, a.ID, a.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("self: %v", err)
	}
	if _, err := s.AddDependency(ctx, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddDependency(ctx, b.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddDependency(ctx, c.ID, a.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cycle: %v", err)
	}
	if err := s.RemoveDependency(ctx, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if blockers, _ := s.Blockers(ctx, a.ID); len(blockers) != 0 {
		t.Fatalf("removed dependency still blocks: %+v", blockers)
	}
	if _, err := s.AddDependency(ctx, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if blockers, _ := s.Blockers(ctx, a.ID); len(blockers) != 1 {
		t.Fatalf("re-added dependency: %+v", blockers)
	}
}

func TestLinksReplicate(t *testing.T) {
	ctx := context.Background()
	src, dst := open(t), open(t)
	p, _ := src.Projects().Create(ctx, Project{Slug: "a", Name: "A"})
	a, _ := src.Tasks().Create(ctx, Task{Title: "A", ProjectID: &p.ID})
	b, _ := src.Tasks().Create(ctx, Task{Title: "B", ProjectID: &p.ID})
	th, _ := src.Threads().Create(ctx, Thread{NodeID: src.log.Node(), ProjectID: &p.ID, Provider: "codex"})
	sync := func() {
		ops, err := src.log.Since(ctx, nil, 10000)
		if err == nil {
			err = dst.log.Apply(ctx, ops)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	src.LinkThread(ctx, th.ID, a.ID, false)
	src.AddDependency(ctx, a.ID, b.ID)
	sync()
	if got, _ := dst.Threads().Get(ctx, th.ID); got.TaskID == nil || *got.TaskID != a.ID {
		t.Fatalf("link not replicated: %+v", got)
	}
	src.UnlinkThread(ctx, th.ID)
	src.RemoveDependency(ctx, a.ID, b.ID)
	sync()
	if got, _ := dst.Threads().Get(ctx, th.ID); got.TaskID != nil {
		t.Fatalf("unlink not replicated: %+v", got)
	}
	if deps, _ := dst.Dependencies().List(ctx, Filter{}); len(deps) != 0 {
		t.Fatalf("removal not replicated: %+v", deps)
	}
}

// Readers that must see every thread, like the cleanup of merged worktrees,
// read past the page size, archived threads included.
func TestListAllReadsPastOnePage(t *testing.T) {
	ctx := t.Context()
	s := open(t)
	const n = 1005
	var last Thread
	for range n {
		th, err := s.Threads().Create(ctx, Thread{NodeID: s.log.Node(), Provider: "codex"})
		if err != nil {
			t.Fatal(err)
		}
		last = th
	}
	if err := s.Threads().Archive(ctx, last.ID); err != nil {
		t.Fatal(err)
	}
	if all, err := s.Threads().ListAll(ctx, Filter{}); err != nil || len(all) != n-1 {
		t.Fatalf("active threads = %d, %v", len(all), err)
	}
	if all, err := s.Threads().ListAll(ctx, Filter{IncludeArchived: true}); err != nil || len(all) != n {
		t.Fatalf("threads with archived = %d, %v", len(all), err)
	}
}
