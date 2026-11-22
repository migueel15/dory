package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/data"
)

func stateOf(t *testing.T, r *Reconciler, id string) string {
	t.Helper()
	st, err := r.Store.SkillStates().List(context.Background(), data.Filter{Fields: map[string]any{"skill_id": id, "node_id": r.Node}})
	if err != nil || len(st) != 1 {
		t.Fatalf("state of %s: %+v %v", id, st, err)
	}
	return st[0].Status
}

func TestRestoreForeignFileAndLink(t *testing.T) {
	ctx := context.Background()
	r, home := setup(t)
	s, _ := Save(ctx, r.Store, data.Skill{Name: "propia", Description: "d", BodyMD: "de Dory"})
	claude := filepath.Join(home, ".claude", "skills", "propia")
	os.MkdirAll(claude, 0o755)
	os.WriteFile(filepath.Join(claude, "SKILL.md"), []byte("a mano"), 0o644)
	// Codex links the folder to another repository.
	elsewhere := filepath.Join(home, "repo", "propia")
	os.MkdirAll(elsewhere, 0o755)
	os.WriteFile(filepath.Join(elsewhere, "SKILL.md"), []byte("del repo"), 0o644)
	codex := filepath.Join(home, ".codex", "skills", "propia")
	os.MkdirAll(filepath.Dir(codex), 0o755)
	os.Symlink(elsewhere, codex)
	r.Reconcile(ctx)
	if got := stateOf(t, r, s.ID); got != "conflict" {
		t.Fatalf("state %s", got)
	}
	conflicts, _ := r.Conflicts(ctx, s.ID)
	kinds := map[string]string{}
	for _, c := range conflicts {
		kinds[c.Harness] = c.Kind
	}
	if kinds["claude"] != "foreign" || kinds["codex"] != "link" {
		t.Fatalf("conflicts %+v", conflicts)
	}
	backup, err := r.Resolve(ctx, s.ID, "restore", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, filepath.Join(claude, "SKILL.md")), "de Dory") || !strings.Contains(read(t, filepath.Join(codex, "SKILL.md")), "de Dory") {
		t.Fatal("catalog version not installed")
	}
	if read(t, filepath.Join(backup, "claude", "propia", "SKILL.md")) != "a mano" || !strings.Contains(read(t, filepath.Join(backup, "codex", "propia.link")), elsewhere) {
		t.Fatal("backup missing")
	}
	if read(t, filepath.Join(elsewhere, "SKILL.md")) != "del repo" {
		t.Fatal("link target touched")
	}
	if got := stateOf(t, r, s.ID); got != "synced" {
		t.Fatalf("state after restore %s", got)
	}
}

func TestAdoptHandEdit(t *testing.T) {
	ctx := context.Background()
	r, home := setup(t)
	s, _ := Save(ctx, r.Store, data.Skill{Name: "propia", Description: "d", BodyMD: "de Dory"})
	r.Reconcile(ctx)
	path := filepath.Join(home, ".claude", "skills", "propia", "SKILL.md")
	os.WriteFile(path, []byte("---\nname: propia\ndescription: nueva\n---\n\nmejorada\n"), 0o644)
	os.WriteFile(filepath.Join(filepath.Dir(path), "extra.md"), []byte("anexo"), 0o644)
	r.Reconcile(ctx)
	if _, err := r.Resolve(ctx, s.ID, "adopt", "claude"); err != nil {
		t.Fatal(err)
	}
	s, _ = r.Store.Skills().Get(ctx, s.ID)
	if s.BodyMD != "mejorada" || s.Description != "nueva" || s.Files["extra.md"] != "anexo" {
		t.Fatalf("catalog %+v", s)
	}
	if !strings.Contains(read(t, filepath.Join(home, ".codex", "skills", "propia", "SKILL.md")), "mejorada") || stateOf(t, r, s.ID) != "synced" {
		t.Fatal("adopted version not installed everywhere")
	}
}

func TestAdoptSystemSkillMakesFork(t *testing.T) {
	ctx := context.Background()
	r, home := setup(t)
	claude := filepath.Join(home, ".claude", "skills", "dory-workflow")
	os.MkdirAll(claude, 0o755)
	os.WriteFile(filepath.Join(claude, "SKILL.md"), []byte("---\nname: dory-workflow\ndescription: \"mi flujo\"\n---\nmío"), 0o644)
	r.Reconcile(ctx)
	id := SystemPrefix + "dory-workflow"
	if _, err := r.Resolve(ctx, id, "adopt", "claude"); err != nil {
		t.Fatal(err)
	}
	sys, _ := r.Store.Skills().Get(ctx, id)
	forks, _ := r.Store.Skills().List(ctx, data.Filter{Fields: map[string]any{"forked_from": id}})
	if sys.DisabledAt == nil || len(forks) != 1 || forks[0].BodyMD != "mío" {
		t.Fatalf("system %+v forks %+v", sys, forks)
	}
	if _, err := os.Stat(claude); !os.IsNotExist(err) {
		t.Fatal("foreign copy left next to the fork")
	}
	if read(t, filepath.Join(home, ".claude", "skills", "dory-workflow-custom", "SKILL.md")) == "" {
		t.Fatal("fork not installed")
	}
}

func TestEditedCopyOfUnwantedSkillIsAConflict(t *testing.T) {
	ctx := context.Background()
	r, home := setup(t)
	s, _ := Save(ctx, r.Store, data.Skill{Name: "propia", Description: "d", BodyMD: "de Dory"})
	r.Reconcile(ctx)
	path := filepath.Join(home, ".claude", "skills", "propia", "SKILL.md")
	os.WriteFile(path, []byte("tocada"), 0o644)
	// Disabled while that node ran an older binary: the edit must not vanish
	// from view.
	SetEnabled(ctx, r.Store, s.ID, false)
	r.Reconcile(ctx)
	if read(t, path) != "tocada" || stateOf(t, r, s.ID) != "conflict" {
		t.Fatalf("edited copy: %q, state %s", read(t, path), stateOf(t, r, s.ID))
	}
	r.Reconcile(ctx) // Still reported on the next pass.
	conflicts, _ := r.Conflicts(ctx, s.ID)
	if len(conflicts) != 1 || !conflicts[0].Orphan || conflicts[0].Harness != "claude" {
		t.Fatalf("conflicts %+v", conflicts)
	}
	if _, err := r.Resolve(ctx, s.ID, "adopt", "claude"); err == nil {
		t.Fatal("adopted the copy of a disabled skill")
	}
	backup, err := r.Resolve(ctx, s.ID, "restore", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) || read(t, filepath.Join(backup, "claude", "propia", "SKILL.md")) != "tocada" {
		t.Fatal("copy not moved to the backup")
	}
	if stateOf(t, r, s.ID) != "removed" {
		t.Fatalf("state %s", stateOf(t, r, s.ID))
	}
}
