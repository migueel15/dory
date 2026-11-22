package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/data"
)

func TestSystemSkillsParse(t *testing.T) {
	shipped, err := System()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, s := range shipped {
		names[s.Name] = true
		if strings.HasPrefix(s.BodyMD, "---") || s.Source != "system" || s.ID != SystemPrefix+s.Name {
			t.Fatalf("bad system skill %+v", s)
		}
	}
	if !names["dory-workflow"] || !names["single-test-run"] {
		t.Fatalf("missing system skills: %v", names)
	}
}

func TestSeedInstallsAndNeverDowngrades(t *testing.T) {
	ctx := context.Background()
	r, home := setup(t)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(home, ".claude", "skills", "dory-workflow", "SKILL.md")); !strings.Contains(got, "# Dory workflow") {
		t.Fatalf("system skill not installed: %q", got)
	}
	// A newer binary already wrote version 99: this one must not go back.
	s, _ := r.Store.Skills().Get(ctx, SystemPrefix+"dory-workflow")
	s.Version, s.BodyMD = 99, "nueva"
	if _, err := r.Store.Skills().Put(ctx, s); err != nil {
		t.Fatal(err)
	}
	if err := SeedSystem(ctx, r.Store); err != nil {
		t.Fatal(err)
	}
	if s, _ = r.Store.Skills().Get(ctx, s.ID); s.BodyMD != "nueva" {
		t.Fatal("an older binary downgraded the system skill")
	}
	// An older version in the catalog is upgraded, keeping it disabled.
	s.Version = 0
	r.Store.Skills().Put(ctx, s)
	SetEnabled(ctx, r.Store, s.ID, false)
	SeedSystem(ctx, r.Store)
	if s, _ = r.Store.Skills().Get(ctx, s.ID); s.Version < 1 || s.BodyMD == "nueva" || s.DisabledAt == nil {
		t.Fatalf("upgrade lost content or state: %+v", s)
	}
}

func TestSystemSkillsAreReadOnlyAndForkable(t *testing.T) {
	ctx := context.Background()
	r, home := setup(t)
	r.Reconcile(ctx)
	id := SystemPrefix + "dory-workflow"
	sys, _ := r.Store.Skills().Get(ctx, id)
	sys.BodyMD = "editada"
	if _, err := Save(ctx, r.Store, sys); err == nil {
		t.Fatal("system skill edited")
	}
	if _, err := Save(ctx, r.Store, data.Skill{Name: "dory-workflow", BodyMD: "x"}); err == nil {
		t.Fatal("system name taken by a user skill")
	}
	fork, err := Fork(ctx, r.Store, id, "")
	if err != nil || fork.Name != "dory-workflow-custom" || fork.ForkedFrom == nil || *fork.ForkedFrom != id {
		t.Fatalf("fork: %+v %v", fork, err)
	}
	if sys, _ = r.Store.Skills().Get(ctx, id); sys.DisabledAt == nil {
		t.Fatal("fork left the original on")
	}
	if _, err = SetEnabled(ctx, r.Store, id, true); err == nil {
		t.Fatal("original enabled while its fork is on")
	}
	r.Reconcile(ctx)
	skills := filepath.Join(home, ".claude", "skills")
	if _, err := os.Stat(filepath.Join(skills, "dory-workflow")); !os.IsNotExist(err) {
		t.Fatal("disabled system skill still installed")
	}
	if read(t, filepath.Join(skills, "dory-workflow-custom", "SKILL.md")) == "" {
		t.Fatal("fork not installed")
	}
	SetEnabled(ctx, r.Store, fork.ID, false)
	if _, err = SetEnabled(ctx, r.Store, id, true); err != nil {
		t.Fatal(err)
	}
	// Enabling the fork again turns the original off.
	SetEnabled(ctx, r.Store, fork.ID, true)
	if sys, _ = r.Store.Skills().Get(ctx, id); sys.DisabledAt == nil {
		t.Fatal("fork and original enabled together")
	}
}
