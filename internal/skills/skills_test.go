package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/nodestate"
	"github.com/migueel15/dory/internal/oplog"
)

func setup(t *testing.T) (*Reconciler, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := localdb.Open(ctx, filepath.Join(dir, "dory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	log, _ := oplog.Open(ctx, db)
	home := filepath.Join(dir, "home")
	return &Reconciler{Store: data.New(db, log), Node: log.Node(), Home: home, State: nodestate.New(db), DataDir: dir,
		Installed: func(context.Context) []string { return []string{"claude", "codex"} }}, home
}

func read(t *testing.T, path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(raw)
}

func TestReconcileWritesUpdatesAndRemoves(t *testing.T) {
	ctx := context.Background()
	r, home := setup(t)
	s, err := Save(ctx, r.Store, data.Skill{Name: "commit-conventions", Description: "Commits", BodyMD: "Usa feat:", Files: map[string]string{"refs/ejemplo.md": "ejemplo"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	claude := filepath.Join(home, ".claude", "skills", "commit-conventions")
	if got := read(t, filepath.Join(claude, "SKILL.md")); !strings.Contains(got, "name: commit-conventions") || !strings.Contains(got, "Usa feat:") {
		t.Fatalf("SKILL.md: %q", got)
	}
	if read(t, filepath.Join(home, ".codex", "skills", "commit-conventions", "refs", "ejemplo.md")) != "ejemplo" {
		t.Fatal("supporting file missing for codex")
	}

	s.BodyMD = "Usa fix:"
	if s, err = Save(ctx, r.Store, s); err != nil || s.Version != 2 {
		t.Fatalf("version %d %v", s.Version, err)
	}
	r.Reconcile(ctx)
	if !strings.Contains(read(t, filepath.Join(claude, "SKILL.md")), "Usa fix:") {
		t.Fatal("update not written")
	}
	states, _ := r.Store.SkillStates().List(ctx, data.Filter{Fields: map[string]any{"skill_id": s.ID}})
	if len(states) != 1 || states[0].Status != "synced" || states[0].Version != 2 {
		t.Fatalf("states %+v", states)
	}

	// Only claude: the codex copy goes away.
	s.Targets.Harnesses = []string{"claude"}
	Save(ctx, r.Store, s)
	r.Reconcile(ctx)
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "commit-conventions")); !os.IsNotExist(err) {
		t.Fatal("codex copy not removed")
	}

	r.Store.Skills().Archive(ctx, s.ID)
	r.Reconcile(ctx)
	if _, err := os.Stat(claude); !os.IsNotExist(err) {
		t.Fatal("archived skill still installed")
	}
}

func TestReconcileNeverOverwritesForeignFiles(t *testing.T) {
	ctx := context.Background()
	r, home := setup(t)
	mine := filepath.Join(home, ".claude", "skills", "propia", "SKILL.md")
	os.MkdirAll(filepath.Dir(mine), 0o755)
	os.WriteFile(mine, []byte("escrita a mano"), 0o644)
	own, _ := Save(ctx, r.Store, data.Skill{Name: "propia", BodyMD: "de Dory"})
	r.Reconcile(ctx)
	if read(t, mine) != "escrita a mano" {
		t.Fatal("foreign file overwritten")
	}
	states, _ := r.Store.SkillStates().List(ctx, data.Filter{Fields: map[string]any{"skill_id": own.ID}})
	if states[0].Status != "conflict" {
		t.Fatalf("state %+v", states[0])
	}

	// A manual edit of a file Dory wrote is drift: kept and reported.
	s, _ := Save(ctx, r.Store, data.Skill{Name: "otra", BodyMD: "v1"})
	r.Reconcile(ctx)
	path := filepath.Join(home, ".codex", "skills", "otra", "SKILL.md")
	os.WriteFile(path, []byte("editada"), 0o644)
	s.BodyMD = "v2"
	Save(ctx, r.Store, s)
	r.Reconcile(ctx)
	if read(t, path) != "editada" {
		t.Fatal("drifted file overwritten")
	}
	if _, err := Save(ctx, r.Store, data.Skill{Name: "Mal Nombre"}); err == nil {
		t.Fatal("bad name accepted")
	}
}
