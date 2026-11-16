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

func TestTaskLifecycle(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	p, err := s.Projects().Put(ctx, Project{Slug: "dory", Name: "Dory"})
	if err != nil {
		t.Fatal(err)
	}
	issue := int64(148)
	task, err := s.Tasks().Put(ctx, Task{ProjectID: &p.ID, Title: "Base local", IssueNumber: &issue})
	if err != nil {
		t.Fatal(err)
	}
	if task.ID == "" || task.Stage != "inbox" || task.CreatedAt == "" || *task.IssueNumber != 148 {
		t.Fatalf("defaults not applied: %+v", task)
	}

	before := opCount(t, s)
	if _, err = s.Tasks().Put(ctx, task); err != nil {
		t.Fatal(err)
	}
	if opCount(t, s) != before {
		t.Fatal("unchanged put wrote an op")
	}
	task.Stage = "ready"
	if task, err = s.Tasks().Put(ctx, task); err != nil || task.Stage != "ready" {
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
	p, _ := s.Projects().Put(ctx, Project{Slug: "dory", Name: "Dory"})
	r, err := s.Repositories().Put(ctx, Repository{ProjectID: p.ID, RemoteURL: "git@github.com:migueel15/dory.git"})
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
	task, _ := s.Tasks().Put(ctx, Task{Title: "Con plan"})
	plan, err := s.Plans().Put(ctx, Plan{TaskID: task.ID, Version: 1, Sections: []byte(`{"objetivo":"SQLite"}`), OpenQuestions: []string{"¿FTS?"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(plan.Sections) != `{"objetivo":"SQLite"}` || plan.OpenQuestions[0] != "¿FTS?" || plan.Author != "agent" {
		t.Fatalf("got %+v", plan)
	}
	m, err := s.Memories().Put(ctx, Memory{Scope: "global", Title: "Decisión", Content: "La v2 usa SQLite", Labels: []string{"v2"}})
	if err != nil || m.Kind != "context" || m.Labels[0] != "v2" {
		t.Fatalf("memory: %+v %v", m, err)
	}
	if _, err = s.Memories().Put(ctx, Memory{Scope: "global", ScopeID: "x", Content: "mal"}); !errors.Is(err, ErrInvalid) {
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
	t2, _ := s.Tasks().Put(ctx, Task{Title: "Dos"})
	t3, _ := s.Tasks().Put(ctx, Task{Title: "Tres"})
	if _, err := s.Dependencies().Put(ctx, Dependency{TaskID: t3.ID, DependsOnID: t2.ID}); err != nil {
		t.Fatal(err)
	}
	blockers, err := s.Blockers(ctx, t3.ID)
	if err != nil || len(blockers) != 1 || blockers[0].ID != t2.ID {
		t.Fatalf("blockers: %+v %v", blockers, err)
	}
	t2.Stage = "cancelled"
	s.Tasks().Put(ctx, t2)
	if blockers, _ = s.Blockers(ctx, t3.ID); len(blockers) != 0 {
		t.Fatalf("cancelled task still blocks: %+v", blockers)
	}
}
