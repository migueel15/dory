package mcp

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/oplog"
)

func memoryStore(t *testing.T) *data.Store {
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

func TestRecallFiltersBeforeLimit(t *testing.T) {
	ctx := t.Context()
	store := memoryStore(t)
	a, _ := store.Projects().Create(ctx, data.Project{Slug: "a", Name: "A"})
	b, _ := store.Projects().Create(ctx, data.Project{Slug: "b", Name: "B"})
	for i := range 150 {
		if _, err := remember(ctx, store, memoryInput{Scope: "project", ScopeID: a.ID, Title: fmt.Sprintf("ruido %d", i), Content: "worktree ajeno worktree"}); err != nil {
			t.Fatal(err)
		}
	}
	want, _ := remember(ctx, store, memoryInput{Scope: "project", ScopeID: b.ID, Title: "buena", Content: "worktree"})
	got, err := recall(ctx, store, recallInput{Query: "worktree", ProjectID: b.ID})
	if err != nil || len(got) != 1 || got[0].ID != want.ID {
		t.Fatalf("recall of project b: %+v, %v", got, err)
	}
	if got, _ = recall(ctx, store, recallInput{Query: "worktree"}); len(got) != recallLimit {
		t.Fatalf("default limit: got %d", len(got))
	}
	if got, _ = recall(ctx, store, recallInput{ID: want.ID}); len(got) != 1 || got[0].Content != "worktree" {
		t.Fatalf("recall by id: %+v", got)
	}
}

func TestRememberUpdatesSameTitle(t *testing.T) {
	ctx := t.Context()
	store := memoryStore(t)
	p, _ := store.Projects().Create(ctx, data.Project{Slug: "p", Name: "P"})
	first, _ := remember(ctx, store, memoryInput{Scope: "project", ScopeID: p.ID, Title: "Regla", Content: "vieja"})
	second, err := remember(ctx, store, memoryInput{Scope: "project", ScopeID: p.ID, Title: "Regla", Content: "nueva"})
	if err != nil || second.ID != first.ID || second.Content != "nueva" {
		t.Fatalf("same title made a new memory: %+v, %v", second, err)
	}
}
