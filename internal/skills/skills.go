// Package skills installs the skills of the catalog in the folders of each
// harness of this node and publishes what really got installed (requirements
// RF-37 to RF-41 of section 5.8). It never touches files it did not write.
package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
)

var namePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Dirs maps each harness to the folder where it reads skills.
func Dirs(home string) map[string]string {
	return map[string]string{
		"claude":      filepath.Join(home, ".claude", "skills"),
		"codex":       filepath.Join(home, ".codex", "skills"),
		"opencode":    filepath.Join(home, ".config", "opencode", "skills"),
		"antigravity": antigravitySkillDir(home),
	}
}

// Save creates or updates a skill, bumping its version when its content changes.
func Save(ctx context.Context, store *data.Store, s data.Skill) (data.Skill, error) {
	if !namePattern.MatchString(s.Name) {
		return s, fmt.Errorf("%w: the name must be kebab-case", data.ErrInvalid)
	}
	for path := range s.Files {
		if path == "SKILL.md" || filepath.IsAbs(path) || strings.Contains(filepath.Clean(path), "..") {
			return s, fmt.Errorf("%w: bad file path %q", data.ErrInvalid, path)
		}
	}
	shipped, err := System()
	if err != nil {
		return s, err
	}
	for _, sys := range shipped {
		if sys.Name == s.Name && s.ID != sys.ID {
			return s, fmt.Errorf("%w: %s es una skill del sistema; haz un fork con otro nombre", data.ErrInvalid, s.Name)
		}
	}
	s.Version, s.Source, s.SystemVersion = 1, "", 0
	if s.ID != "" {
		if old, err := store.Skills().Get(ctx, s.ID); err == nil {
			if old.Source == "system" {
				return old, fmt.Errorf("%w: las skills del sistema no se pueden editar; haz un fork", data.ErrInvalid)
			}
			// Activation and the fork link change only through SetEnabled and Fork.
			s.ForkedFrom, s.DisabledAt = old.ForkedFrom, old.DisabledAt
			s.Version = old.Version
			if old.BodyMD != s.BodyMD || old.Description != s.Description || !maps(old.Files, s.Files) || old.Name != s.Name {
				s.Version++
			}
		}
	}
	return store.Skills().Put(ctx, s)
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
	Store *data.Store
	Node  string
	Home  string
	// Manifest is the file where the reconciler remembers what it wrote.
	Manifest string
	// Installed reports which harnesses this machine has.
	Installed func(ctx context.Context) []string

	mu   sync.Mutex
	wake chan struct{}
	// run keeps one reconcile, or one conflict resolution, at a time.
	run sync.Mutex
}

// manifest maps each file the reconciler wrote to the hash it wrote.
type manifest map[string]string

func (r *Reconciler) load() manifest {
	m := manifest{}
	raw, err := os.ReadFile(r.Manifest)
	if err == nil {
		json.Unmarshal(raw, &m)
	}
	return m
}

func (r *Reconciler) save(m manifest) error {
	raw, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(r.Manifest, raw, 0o600)
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
	all, err := r.Store.Skills().List(ctx, data.Filter{Limit: 1000, IncludeArchived: true})
	if err != nil {
		return err
	}
	installed := r.Installed(ctx)
	dirs := Dirs(r.Home)
	m := r.load()
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
			dir := filepath.Join(dirs[harness], s.Name)
			if harness == "antigravity" {
				dir, err = r.antigravityTarget(dir, s.Name, m, want)
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
				st.Message = strings.TrimSpace(st.Message + "\nconflicto: " + path + " se editó a mano y Dory no la quita")
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
	if err = r.save(m); err != nil {
		return err
	}
	return r.publish(ctx, states)
}

// roots are the folders that hold one folder per skill on this node.
func (r *Reconciler) roots() []string {
	out := []string{filepath.Join(filepath.Dir(r.Manifest), "antigravity-skills")}
	for _, dir := range Dirs(r.Home) {
		out = append(out, dir)
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
			return "conflicto: " + path + " no lo escribió Dory o se editó a mano"
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "error: " + err.Error()
	}
	if info, err := os.Lstat(filepath.Dir(path)); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "conflicto: " + filepath.Dir(path) + " es un enlace simbólico ajeno"
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
	current, err := r.Store.SkillStates().List(ctx, data.Filter{Fields: map[string]any{"node_id": r.Node}, Limit: 1000, IncludeArchived: true})
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
		if _, err = r.Store.SkillStates().Put(ctx, st); err != nil {
			return err
		}
	}
	return nil
}
