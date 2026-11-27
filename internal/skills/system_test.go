package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
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
	if !names["dory-workflow"] || !names["single-test-run"] || !names["dory-issue"] {
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
	s.SystemVersion, s.BodyMD = 99, "nueva"
	if _, err := r.Store.Skills().Patch(ctx, s.ID, data.Fields{"system_version": s.SystemVersion, "body_md": s.BodyMD}); err != nil {
		t.Fatal(err)
	}
	if err := SeedSystem(ctx, r.Store); err != nil {
		t.Fatal(err)
	}
	if s, _ = r.Store.Skills().Get(ctx, s.ID); s.BodyMD != "nueva" {
		t.Fatal("an older binary downgraded the system skill")
	}
	// An older version in the catalog is upgraded, keeping it disabled.
	s.SystemVersion = 0
	r.Store.Skills().Patch(ctx, s.ID, data.Fields{"system_version": s.SystemVersion})
	SetEnabled(ctx, r.Store, s.ID, false)
	SeedSystem(ctx, r.Store)
	if s, _ = r.Store.Skills().Get(ctx, s.ID); s.SystemVersion < 1 || s.BodyMD == "nueva" || s.DisabledAt == nil {
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

// TestSystemSkillVersionsLocked fails when a system skill changes without a
// new version: nodes that already have that version would never update it.
// After changing a skill, bump its version and update system.lock.json.
func TestSystemSkillVersionsLocked(t *testing.T) {
	shipped, _ := System()
	raw, err := os.ReadFile("system.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	type entry struct {
		Version int64  `json:"version"`
		SHA256  string `json:"sha256"`
	}
	lock := map[string]entry{}
	if err = json.Unmarshal(raw, &lock); err != nil {
		t.Fatal(err)
	}
	for _, s := range shipped {
		h := sha256.New()
		h.Write([]byte(s.Description + "\x00" + s.BodyMD))
		paths := make([]string, 0, len(s.Files))
		for p := range s.Files {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		for _, p := range paths {
			h.Write([]byte("\x00" + p + "\x00" + s.Files[p]))
		}
		sum := hex.EncodeToString(h.Sum(nil))
		got, ok := lock[s.Name]
		switch {
		case !ok || got.Version > s.Version:
			t.Errorf("%s: add it to system.lock.json as {\"version\": %d, \"sha256\": %q}", s.Name, s.Version, sum)
		case got.Version == s.Version && got.SHA256 != sum:
			t.Errorf("%s changed without a new version: bump its version to %d and set sha256 %q in system.lock.json", s.Name, s.Version+1, sum)
		case got.Version < s.Version:
			t.Errorf("%s: update system.lock.json to {\"version\": %d, \"sha256\": %q}", s.Name, s.Version, sum)
		}
	}
}

func TestSeedRestoresEditsOfOldBinaries(t *testing.T) {
	ctx := context.Background()
	r, _ := setup(t)
	SeedSystem(ctx, r.Store)
	// A binary without system skills edits it: its Save bumps version but
	// knows nothing of system_version.
	s, _ := r.Store.Skills().Get(ctx, SystemPrefix+"dory-workflow")
	s.BodyMD, s.Version = "pisada", s.Version+1
	r.Store.Skills().Patch(ctx, s.ID, data.Fields{"body_md": s.BodyMD, "version": s.Version})
	SeedSystem(ctx, r.Store)
	if s, _ = r.Store.Skills().Get(ctx, s.ID); s.BodyMD == "pisada" || s.Version <= 2 {
		t.Fatalf("old edit kept: %+v", s)
	}
}

func TestSeedTakesItsNameFromAUserSkill(t *testing.T) {
	ctx := context.Background()
	r, _ := setup(t)
	// A user skill with the same name, added before system skills existed.
	user, err := r.Store.Skills().Create(ctx, data.Skill{Name: "dory-workflow", Description: "mía", BodyMD: "propia"})
	if err != nil {
		t.Fatal(err)
	}
	if err = SeedSystem(ctx, r.Store); err != nil {
		t.Fatal(err)
	}
	sys, _ := r.Store.Skills().Get(ctx, SystemPrefix+"dory-workflow")
	user, _ = r.Store.Skills().Get(ctx, user.ID)
	if sys.Name != "dory-workflow" || user.Name != "dory-workflow-custom" || user.ForkedFrom == nil || *user.ForkedFrom != sys.ID {
		t.Fatalf("names: system %q, user %q", sys.Name, user.Name)
	}
	if sys.DisabledAt == nil || user.DisabledAt != nil {
		t.Fatal("the user's skill must stay on and the system one off")
	}
}
