// Package skills installs the skills of the catalog in the folders of each
// harness of this node and publishes what really got installed (requirements
// RF-37 to RF-41 of section 5.8). It never touches files it did not write.
package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/migueel15/dory/internal/i18n"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/nodestate"
)

var namePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Save creates or updates a skill, bumping its version when its content changes.
func Save(ctx context.Context, store *data.Store, s data.Skill) (data.Skill, error) {
	if !namePattern.MatchString(s.Name) {
		return s, data.ErrInvalid.Msgf("the name must be kebab-case")
	}
	for path := range s.Files {
		if path == "SKILL.md" || filepath.IsAbs(path) || strings.Contains(filepath.Clean(path), "..") {
			return s, data.ErrInvalid.Msgf("bad file path %q", path)
		}
	}
	shipped, err := System()
	if err != nil {
		return s, err
	}
	for _, sys := range shipped {
		if sys.Name == s.Name && s.ID != sys.ID {
			return s, data.ErrInvalid.Msgf("%s is a system skill; fork it under another name", s.Name).With("reason", "system_skill_name").With("name", s.Name)
		}
	}
	s.Version, s.Source, s.SystemVersion = 1, "", 0
	if s.ID == "" {
		return store.Skills().Create(ctx, s)
	}
	old, err := store.Skills().Get(ctx, s.ID)
	if err != nil {
		return old, err
	}
	if old.Source == "system" {
		return old, data.ErrInvalid.Msgf("system skills cannot be edited; fork it").With("reason", "system_skill_readonly")
	}
	s.Version = old.Version
	if old.BodyMD != s.BodyMD || old.Description != s.Description || !maps(old.Files, s.Files) || old.Name != s.Name {
		s.Version++
	}
	if s.Files == nil {
		s.Files = map[string]string{}
	}
	return store.Skills().PatchIf(ctx, s.ID, old.UpdatedAt, data.Fields{
		"name": s.Name, "description": s.Description, "body_md": s.BodyMD,
		"files": s.Files, "targets": s.Targets, "version": s.Version,
	})
}

func maps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// render is the SKILL.md a harness reads: frontmatter plus body.
func render(s data.Skill) string {
	return fmt.Sprintf("---\nname: %s\ndescription: %q\n---\n\n%s\n", s.Name, s.Description, strings.TrimSpace(s.BodyMD))
}

// Reconciler keeps this node's harness folders in line with the catalog.
type Reconciler struct {
	Store     *data.Store
	Node      string
	Home      string
	Locations func() map[string]harness.SkillLocation
	// State remembers the files written by this reconciler.
	State *nodestate.Store
	// DataDir holds managed Antigravity skills and backups.
	DataDir string
	// Installed reports which harnesses this machine has.
	Installed func(ctx context.Context) []string

	mu   sync.Mutex
	wake chan struct{}
	// run keeps one reconcile, or one conflict resolution, at a time.
	run sync.Mutex
}

// manifest maps each file the reconciler wrote to the hash it wrote.
type manifest map[string]string

func (r *Reconciler) load(ctx context.Context) (manifest, error) {
	var m manifest
	if _, err := r.State.Get(ctx, nodestate.KeySkillsManifest, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = make(manifest)
	}
	return m, nil
}

func (r *Reconciler) save(ctx context.Context, m manifest) error {
	return r.State.Put(ctx, nodestate.KeySkillsManifest, m)
}

func hash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// Changed asks for a reconcile soon; called on every data change.
func (r *Reconciler) Changed(entity string) {
	if entity != "skill" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.wake == nil {
		r.wake = make(chan struct{}, 1)
	}
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// Run reconciles at start, on each catalog change and every few minutes.
func (r *Reconciler) Run(ctx context.Context) {
	r.Changed("skill")
	for {
		r.mu.Lock()
		wake := r.wake
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-wake:
		case <-time.After(5 * time.Minute):
		}
		if err := r.Reconcile(ctx); err != nil {
			log.Printf("skills: %v", err)
		}
	}
}

// Reconcile writes, updates and removes skill files, then publishes the
// state of each skill on this node.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	r.run.Lock()
	defer r.run.Unlock()
	return r.reconcile(ctx)
}

func (r *Reconciler) reconcile(ctx context.Context) error {
	// Seeding writes only when this binary ships something newer, so the
	// catalog change it causes ends in a reconcile that seeds nothing.
	if err := SeedSystem(ctx, r.Store); err != nil {
		log.Printf("skills: system skills: %v", err)
	}
	all, err := r.Store.Skills().ListAll(ctx, data.Filter{IncludeArchived: true})
	if err != nil {
		return err
	}
	installed := r.Installed(ctx)
	dirs := r.locations()
	m, err := r.load(ctx)
	if err != nil {
		return err
	}
	want := map[string]bool{}
	states := map[string]data.SkillState{}
	for _, s := range all {
		st := data.SkillState{SkillID: s.ID, NodeID: r.Node, Status: "synced", Version: s.Version}
		targeted := s.ArchivedAt == nil && s.DisabledAt == nil && (len(s.Targets.Nodes) == 0 || slices.Contains(s.Targets.Nodes, r.Node))
		if !targeted {
			st.Status, st.Version = "removed", 0
			states[s.ID] = st
			continue
		}
		var problems []string
		for _, harness := range installed {
			if len(s.Targets.Harnesses) > 0 && !slices.Contains(s.Targets.Harnesses, harness) {
				continue
			}
			target, ok := dirs[harness]
			if !ok {
				continue
			}
			dir := filepath.Join(target.Dir, s.Name)
			if target.ManagedSubdir != "" {
				dir, err = r.linkTarget(dir, filepath.Join(r.DataDir, target.ManagedSubdir, s.Name), m, want)
				if err != nil {
					problems = append(problems, err.Error())
					if !strings.HasPrefix(err.Error(), "conflicto:") {
						st.Status = "error"
					} else if st.Status != "error" {
						st.Status = "conflict"
					}
					continue
				}
			}
			files := map[string]string{"SKILL.md": render(s)}
			for p, c := range s.Files {
				files[p] = c
			}
			for p, content := range files {
				path := filepath.Join(dir, filepath.FromSlash(p))
				want[path] = true
				if msg := r.write(path, []byte(content), m); msg != "" {
					problems = append(problems, msg)
					if strings.HasPrefix(msg, "conflicto") && st.Status != "error" {
						st.Status = "conflict"
					} else {
						st.Status = "error"
					}
				}
			}
		}
		sort.Strings(problems)
		st.Message = strings.Join(problems, "\n")
		states[s.ID] = st
	}
	// Files written before that no skill wants any more are removed. One
	// edited since is kept and stays tracked, and its skill is reported in
	// conflict so the user can resolve it.
	byName := map[string]string{}
	for _, s := range all {
		byName[s.Name] = s.ID
	}
	for path, h := range m {
		if want[path] {
			continue
		}
		if target, ok := strings.CutPrefix(h, linkPrefix); ok {
			if actual, err := os.Readlink(path); err == nil && actual == target {
				if err := os.Remove(path); err != nil {
					continue
				}
			}
			delete(m, path)
			continue
		}
		raw, err := os.ReadFile(path)
		if err == nil && hash(raw) != h {
			if id, ok := byName[r.ownerName(path)]; ok {
				st := states[id]
				st.SkillID, st.NodeID, st.Status = id, r.Node, "conflict"
				st.Message = strings.TrimSpace(st.Message + "\n" + i18n.Textf("skills.conflict_edited_left", path))
				states[id] = st
				continue
			}
		}
		if err == nil && hash(raw) == h {
			os.Remove(path)
			removeEmpty(filepath.Dir(path), r.Home)
		}
		if err == nil || errors.Is(err, os.ErrNotExist) {
			delete(m, path)
		}
	}
	if err = r.save(ctx, m); err != nil {
		return err
	}
	return r.publish(ctx, states)
}

// roots are the folders that hold one folder per skill on this node.
func (r *Reconciler) roots() []string {
	var out []string
	for _, loc := range r.locations() {
		out = append(out, loc.Dir)
		if loc.ManagedSubdir != "" {
			out = append(out, filepath.Join(r.DataDir, loc.ManagedSubdir))
		}
	}
	return out
}

// ownerName is the skill folder a file belongs to, or "".
func (r *Reconciler) ownerName(path string) string {
	for _, root := range r.roots() {
		if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
			return strings.Split(filepath.ToSlash(rel), "/")[0]
		}
	}
	return ""
}

// write puts content at path when the path is free or holds what Dory wrote
// last time. It returns a message when it cannot.
func (r *Reconciler) write(path string, content []byte, m manifest) string {
	current, err := os.ReadFile(path)
	if err == nil {
		if hash(current) == hash(content) {
			m[path] = hash(content)
			return ""
		}
		if prev, ours := m[path]; !ours || prev != hash(current) {
			return i18n.Textf("skills.conflict_not_ours", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "error: " + err.Error()
	}
	if info, err := os.Lstat(filepath.Dir(path)); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return i18n.Textf("skills.conflict_foreign_symlink", filepath.Dir(path))
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "error: " + err.Error()
	}
	if err = os.WriteFile(path, content, 0o644); err != nil {
		return "error: " + err.Error()
	}
	m[path] = hash(content)
	return ""
}

func removeEmpty(dir, stop string) {
	for dir != stop && strings.HasPrefix(dir, stop) {
		if os.Remove(dir) != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

// publish writes this node's states, only where they changed.
func (r *Reconciler) publish(ctx context.Context, states map[string]data.SkillState) error {
	current, err := r.Store.SkillStates().ListAll(ctx, data.Filter{Fields: map[string]any{"node_id": r.Node}, IncludeArchived: true})
	if err != nil {
		return err
	}
	have := map[string]data.SkillState{}
	for _, s := range current {
		have[s.SkillID] = s
	}
	for id, st := range states {
		if old, ok := have[id]; ok && old.Status == st.Status && old.Version == st.Version && old.Message == st.Message {
			continue
		}
		if _, err = r.Store.SkillStates().Upsert(ctx, st); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) locations() map[string]harness.SkillLocation {
	if r.Locations == nil {
		return nil
	}
	return r.Locations()
}
