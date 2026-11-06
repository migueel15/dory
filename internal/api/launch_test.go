package api

import (
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/service"
)

func TestLaunchPrompt(t *testing.T) {
	item := service.Item{Title: "Idea"}
	item.ID = "12345678-aaaa"
	triage := launchPrompt(item, roleTriage, false, "", "")
	if !strings.Contains(triage, "No cambies el código") {
		t.Fatalf("triage prompt allows code changes: %s", triage)
	}
	if !strings.Contains(triage, "pásala a ready") || !strings.Contains(triage, "upsert_feature") {
		t.Fatalf("triage prompt does not define the item: %s", triage)
	}
	for _, base := range []string{"", "main"} {
		p := launchPrompt(item, roleImplementation, true, base, "")
		from := "origin/HEAD"
		if base != "" {
			from = "la rama " + base
		}
		for _, instruction := range []string{
			from, "dory/12345678", "~/.t3/worktrees/<repo>/dory-12345678",
			"nombre de la carpeta del repositorio principal", "Tras el merge de la PR",
			"vuelve al checkout principal", "git worktree remove", "git branch -d",
			"sin cambios pendientes", "No fuerces la limpieza", "explica el motivo en este hilo",
		} {
			if !strings.Contains(p, instruction) {
				t.Errorf("worktree prompt missing %q: %s", instruction, p)
			}
		}
	}
	if p := launchPrompt(item, roleImplementation, false, "", ""); strings.Contains(p, "worktree") {
		t.Fatalf("unexpected worktree: %s", p)
	}
	if p := launchPrompt(item, roleReplan, false, "", "añade tests"); !strings.Contains(p, "añade tests") || !strings.Contains(p, "No cambies el código") || strings.Contains(p, "Implementa este item") {
		t.Fatalf("replan prompt wrong: %s", p)
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
