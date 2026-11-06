package service

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/migueel15/dory/migrations"
	"github.com/pressly/goose/v3"
)

func TestPlanMarkdownSkipsEmptySections(t *testing.T) {
	got := PlanSections{Goal: "Hacer X", Validation: " go test ./... "}.Markdown()
	want := "## Objetivo\nHacer X\n\n## Cómo probar\ngo test ./..."
	if got != want {
		t.Fatalf("Markdown() = %q, want %q", got, want)
	}
	if !(PlanSections{Risks: "  "}).empty() {
		t.Fatal("blank sections must count as empty")
	}
}

// TestPlanLifecycle needs DORY_TEST_DATABASE_URL pointing at an empty database.
func TestPlanLifecycle(t *testing.T) {
	url := os.Getenv("DORY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("DORY_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	goose.SetBaseFS(migrations.FS)
	if err = goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err = goose.UpContext(ctx, stdlib.OpenDBFromPool(pool), "."); err != nil {
		t.Fatal(err)
	}
	svc := New(pool)
	item, err := svc.Items().Put(ctx, "test", Item{Title: "Idea", Status: "inbox"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = AcceptPlan(ctx, svc, "test", item.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("accepting without a plan = %v", err)
	}
	v1, err := ProposePlan(ctx, svc, "test", item.ID, "agent", PlanSections{Goal: "G1"}, []string{"¿Dónde?", " "})
	if err != nil {
		t.Fatal(err)
	}
	if v1.Version != 1 || len(v1.OpenQuestions) != 1 {
		t.Fatalf("v1 = %+v", v1)
	}
	if got, _ := svc.Items().Get(ctx, item.ID); got.Status != "triaged" {
		t.Fatalf("proposing moved the item to %s, want triaged", got.Status)
	}
	if _, _, err = AcceptPlan(ctx, svc, "test", item.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("accepting with open questions = %v", err)
	}
	v2, err := ProposePlan(ctx, svc, "test", item.ID, "user", PlanSections{Goal: "G2", Validation: "V"}, nil)
	if err != nil || v2.Version != 2 {
		t.Fatalf("v2 = %+v, %v", v2, err)
	}
	accepted, plan, err := AcceptPlan(ctx, svc, "test", item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Status != "ready" || accepted.BodyMd != "## Objetivo\nG2\n\n## Cómo probar\nV" || plan.Version != 2 || plan.AcceptedAt == nil {
		t.Fatalf("accepted = %+v, plan = %+v", accepted, plan)
	}
	if again, err := svc.ItemPlans().Put(ctx, "test", ItemPlan{Record: Record{ID: v1.ID}, ItemID: item.ID, Sections: PlanSections{Goal: "changed"}}); err != nil || again.Sections.Goal != "G1" || again.Version != 1 {
		t.Fatalf("an existing version must stay immutable: %+v, %v", again, err)
	}
	if _, err = ProposePlan(ctx, svc, "test", item.ID, "agent", PlanSections{Goal: "G3"}, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.Items().Get(ctx, item.ID); got.Status != "triaged" {
		t.Fatalf("a new version after acceptance left the item in %s", got.Status)
	}
	got, err := LatestPlan(ctx, svc, item.ID)
	if err != nil || got.Version != 3 || got.AcceptedAt != nil {
		t.Fatalf("latest = %+v, %v", got, err)
	}
	item.Status = "in_progress"
	if _, err = svc.Items().Put(ctx, "test", item); err != nil {
		t.Fatal(err)
	}
	if _, err = ProposePlan(ctx, svc, "test", item.ID, "agent", PlanSections{Goal: "G4"}, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("proposing on an item in progress = %v", err)
	}
}
