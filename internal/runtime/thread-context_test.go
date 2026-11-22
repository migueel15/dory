package runtime

import (
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/data"
)

func TestThreadContext(t *testing.T) {
	ctx := t.Context()
	m, store := manager(t, 0)
	p, _ := store.Projects().Put(ctx, data.Project{Slug: "dory", Name: "Dory"})
	if _, err := store.Repositories().Put(ctx, data.Repository{ProjectID: p.ID, RemoteURL: "https://github.com/x/dory.git", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	issue := int64(7)
	task, _ := store.Tasks().Put(ctx, data.Task{ProjectID: &p.ID, Title: "Ficha", Stage: "inbox", IssueNumber: &issue})
	got := m.threadContext(ctx, data.Thread{ID: "th", ProjectID: &p.ID, TaskID: &task.ID, Branch: "feat/7-ficha"}, "/w")
	for _, want := range []string{"Project dory", "x/dory.git", "branch feat/7-ficha", "Task " + task.ID, "issue #7"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	if got = m.threadContext(ctx, data.Thread{ID: "th", ProjectID: &p.ID}, ""); !strings.Contains(got, "No task linked") {
		t.Fatalf("thread without task: %q", got)
	}
}
