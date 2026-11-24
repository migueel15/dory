package app

import (
	"testing"

	"github.com/migueel15/dory/internal/data"
)

func TestSessionStart(t *testing.T) {
	ctx := t.Context()
	store := memoryStore(t)
	mem := &Memories{Store: store}
	projects := &Projects{Store: store}
	a, _ := store.Projects().Create(ctx, data.Project{Slug: "a", Name: "A"})
	store.Projects().Create(ctx, data.Project{Slug: "b", Name: "B"})
	mem.Remember(ctx, RememberInput{Title: "Global", Content: "g"})
	mem.Remember(ctx, RememberInput{Scope: "project", ScopeID: a.ID, Title: "De A", Content: "x"})
	open, _ := store.Tasks().Create(ctx, data.Task{Title: "Abierta", ProjectID: &a.ID})
	store.Tasks().Create(ctx, data.Task{Title: "Hecha", ProjectID: &a.ID, Stage: "cancelled"})
	s := &Session{Node: "n1", Tasks: &Tasks{Store: store, Plans: &Plans{Store: store}}, Memories: mem, Projects: projects}

	out, err := s.Start(ctx, FindInput{Project: "zzz"})
	if err != nil || out["project_match"] != "none" || len(out["memories"].([]MemoryEntry)) != 1 {
		t.Fatalf("unknown project: %v %v", out, err)
	}
	out, err = s.Start(ctx, FindInput{Project: "a"})
	if err != nil {
		t.Fatal(err)
	}
	p := out["project"].(map[string]any)
	tasks, memories := p["open_tasks"].([]TaskEntry), p["memories"].([]MemoryEntry)
	if len(tasks) != 1 || tasks[0].ID != open.ID || len(memories) != 1 || memories[0].Title != "De A" {
		t.Fatalf("project data: %+v %+v", tasks, memories)
	}
}
