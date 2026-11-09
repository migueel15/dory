package api

import (
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/service"
)

func TestLaunchPrompt(t *testing.T) {
	item := service.Item{Title: "Idea"}
	item.ID = "12345678-aaaa"
	p := launchPrompt(item, "/w/dory-x")
	for _, instruction := range []string{"worktree sin rama en /w/dory-x", "git switch -c", "No crees otro worktree", "Dory elimina el worktree"} {
		if !strings.Contains(p, instruction) {
			t.Errorf("worktree prompt missing %q: %s", instruction, p)
		}
	}
	if strings.Contains(p, "dory/") || strings.Contains(p, "git worktree add") {
		t.Fatalf("prompt must not impose branch names or ask for a worktree: %s", p)
	}
	if p := launchPrompt(item, ""); strings.Contains(p, "worktree") {
		t.Fatalf("unexpected worktree: %s", p)
	}
}

func TestRefineFirstMessage(t *testing.T) {
	item := service.Item{Title: "Idea", RawText: "botones"}
	item.ID = "12345678-aaaa"
	msg := refineFirstMessage(item, "")
	if !strings.Contains(msg, "propose_plan") || !strings.Contains(msg, "No cambies el código") || strings.Contains(msg, "Indicaciones del usuario") {
		t.Fatalf("unexpected first message: %s", msg)
	}
	if strings.Contains(msg, "move_item") || strings.Contains(msg, "pásala a ready") {
		t.Fatalf("the definition agent must not move the item: %s", msg)
	}
	if msg := refineFirstMessage(item, "usa el color de marca"); !strings.HasSuffix(msg, "Indicaciones del usuario:\nusa el color de marca") {
		t.Fatalf("user text missing: %s", msg)
	}
}
