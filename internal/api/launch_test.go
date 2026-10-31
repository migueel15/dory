package api

import (
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/service"
)

func TestLaunchPrompt(t *testing.T) {
	item := service.Item{Title: "Idea"}
	item.ID = "12345678-aaaa"
	if p := launchPrompt(item, roleTriage, false); !strings.Contains(p, "No cambies nada") {
		t.Fatalf("triage prompt allows changes: %s", p)
	}
	if p := launchPrompt(item, roleImplementation, true); !strings.Contains(p, "dory/12345678") {
		t.Fatalf("worktree branch missing: %s", p)
	}
	if p := launchPrompt(item, roleImplementation, false); strings.Contains(p, "worktree") {
		t.Fatalf("unexpected worktree: %s", p)
	}
}
