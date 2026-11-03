package api

import (
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/service"
)

func TestLaunchPrompt(t *testing.T) {
	item := service.Item{Title: "Idea"}
	item.ID = "12345678-aaaa"
	triage := launchPrompt(item, roleTriage, false, "")
	if !strings.Contains(triage, "No cambies el código") {
		t.Fatalf("triage prompt allows code changes: %s", triage)
	}
	if !strings.Contains(triage, "pásala a ready") || !strings.Contains(triage, "upsert_feature") {
		t.Fatalf("triage prompt does not define the item: %s", triage)
	}
	if p := launchPrompt(item, roleImplementation, true, ""); !strings.Contains(p, "dory/12345678") || !strings.Contains(p, "origin/HEAD") {
		t.Fatalf("worktree branch missing: %s", p)
	}
	if p := launchPrompt(item, roleImplementation, false, ""); strings.Contains(p, "worktree") {
		t.Fatalf("unexpected worktree: %s", p)
	}
}
