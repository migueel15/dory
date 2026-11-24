package app

import (
	"context"
	"testing"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
)

func TestPullReviewsMarksAndAsk(t *testing.T) {
	ctx := context.Background()
	tasks, _ := newTasks(t)
	r := &testRuntime{}
	p := &PullReviews{Store: tasks.Store, Threads: &Threads{Runtime: r, Settings: testSettings{}}}
	if err := p.Mark(ctx, "o/r", 7, "a.go", "h1"); err != nil {
		t.Fatal(err)
	}
	if err := p.Mark(ctx, "o/r", 7, "a.go", "h2"); err != nil {
		t.Fatal(err)
	}
	if marks, err := p.Marks(ctx, "o/r", 7); err != nil || len(marks) != 1 || marks[0].PatchHash != "h2" {
		t.Fatalf("marks = %+v, %v", marks, err)
	}
	if err := p.Unmark(ctx, "o/r", 7, "a.go"); err != nil {
		t.Fatal(err)
	}
	if marks, _ := p.Marks(ctx, "o/r", 7); len(marks) != 0 {
		t.Fatalf("marks after unmark = %+v", marks)
	}
	// Without a review thread, the first question must say how to launch it.
	if _, err := p.Ask(ctx, "o/r", 7, PullQuestion{Question: "¿Qué hace?"}); apperr.CodeOf(err) != apperr.NoThread {
		t.Fatalf("ask without launch = %v", err)
	}
	// With one, a follow-up question goes to it as typed.
	th, err := tasks.Store.Threads().Create(ctx, data.Thread{NodeID: "n", Provider: "codex", Title: pullTitle("o/r", 7)})
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Ask(ctx, "o/r", 7, PullQuestion{Question: "¿Y esto?"})
	if err != nil || got.ID != th.ID {
		t.Fatalf("ask = %+v, %v", got, err)
	}
}
