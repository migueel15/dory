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

func TestItemWorktree(t *testing.T) {
	for _, tc := range []struct{ root, want string }{
		{"/home/miguel/repos/dory", "/home/miguel/.t3/worktrees/dory/dory-3e3f909c"},
		{"/Users/ana/src/dory", "/Users/ana/.t3/worktrees/dory/dory-3e3f909c"},
	} {
		item := service.Item{}
		item.ID = "3e3f909c-bf6f-4e6b-b524-a152053f84f6"
		got, create := itemWorktree(item, tc.root)
		if got != tc.want || !create {
			t.Fatalf("path=%q create=%v", got, create)
		}
		item.ID = "abcdef12-bf6f-4e6b-b524-a152053f84f6"
		other, create := itemWorktree(item, tc.root)
		if other == got || !create {
			t.Fatalf("items must use distinct paths: %q", other)
		}
		item.Refs = []string{"https://github.com/migueel15/dory/issues/122", "worktree:" + got}
		reused, create := itemWorktree(item, tc.root)
		if reused != got || create {
			t.Fatalf("must reuse recorded path: %q create=%v", reused, create)
		}
	}
}
