package skills

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/data"
)

// Conflict is a copy of a catalog skill on this node that Dory did not write
// or someone edited: a foreign link, a foreign file or a hand edit.
type Conflict struct {
	Harness string `json:"harness"`
	Path    string `json:"path"`
	// Kind is "link" (the folder links elsewhere), "foreign" (Dory never
	// wrote the file) or "edited" (Dory wrote it and someone changed it).
	Kind string `json:"kind"`
	// SkillMD is the node's SKILL.md, so the user can compare before choosing.
	SkillMD string `json:"skill_md"`
}

// maxAdopt bounds what adopting a node copy reads into the catalog.
const maxAdopt = 1 << 20

// Conflicts lists the copies of a skill on this node that block its install.
func (r *Reconciler) Conflicts(ctx context.Context, id string) ([]Conflict, error) {
	s, err := r.Store.Skills().Get(ctx, id)
	if err != nil {
		return nil, err
	}
	m := r.load()
	out := []Conflict{}
	for _, harness := range r.Installed(ctx) {
		if len(s.Targets.Harnesses) > 0 && !slices.Contains(s.Targets.Harnesses, harness) {
			continue
		}
		dir := filepath.Join(Dirs(r.Home)[harness], s.Name)
		if c, ok := r.conflict(harness, dir, s, m); ok {
			out = append(out, c)
		}
	}
	return out, nil
}

// conflict checks one harness folder of a skill.
func (r *Reconciler) conflict(harness, dir string, s data.Skill, m manifest) (Conflict, bool) {
	c := Conflict{Harness: harness, Path: dir}
	if raw, err := os.ReadFile(filepath.Join(dir, "SKILL.md")); err == nil {
		c.SkillMD = string(raw)
	}
	if target, err := os.Readlink(dir); err == nil {
		if m[dir] == linkPrefix+target {
			dir, c.Path = target, target // Dory's own link to its managed copy.
		} else {
			c.Kind = "link"
			return c, true
		}
	}
	files := map[string]string{"SKILL.md": render(s)}
	for p, content := range s.Files {
		files[p] = content
	}
	for p, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(p))
		current, err := os.ReadFile(path)
		if err != nil || hash(current) == hash([]byte(content)) {
			continue
		}
		if prev, ours := m[path]; !ours {
			c.Kind = "foreign"
		} else if prev != hash(current) && c.Kind == "" {
			c.Kind = "edited"
		}
	}
	return c, c.Kind != ""
}

// Resolve settles the conflicts of a skill on this node. "restore" keeps the
// catalog version; "adopt" first takes the copy of the given harness into the
// catalog (for a system skill, into a new fork that replaces it). In both
// cases the node's copies are moved to a backup folder and Dory installs the
// catalog version, so the skill is synced again.
func (r *Reconciler) Resolve(ctx context.Context, id, action, harness string) (string, error) {
	s, err := r.Store.Skills().Get(ctx, id)
	if err != nil {
		return "", err
	}
	conflicts, err := r.Conflicts(ctx, id)
	if err != nil {
		return "", err
	}
	if len(conflicts) == 0 {
		return "", fmt.Errorf("%w: la skill no tiene conflictos en este nodo", data.ErrInvalid)
	}
	switch action {
	case "restore":
	case "adopt":
		i := slices.IndexFunc(conflicts, func(c Conflict) bool { return c.Harness == harness })
		if i < 0 {
			return "", fmt.Errorf("%w: no hay conflicto en %s", data.ErrInvalid, harness)
		}
		if err = r.adopt(ctx, s, conflicts[i].Path); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("%w: action must be restore or adopt", data.ErrInvalid)
	}
	r.run.Lock()
	defer r.run.Unlock()
	backup := filepath.Join(filepath.Dir(r.Manifest), "skill-backups", time.Now().UTC().Format("20060102-150405"))
	m := r.load()
	for _, c := range conflicts {
		if err = moveAside(c.Path, filepath.Join(backup, c.Harness, filepath.Base(c.Path))); err != nil {
			return "", err
		}
		for path := range m {
			if path == c.Path || strings.HasPrefix(path, c.Path+string(filepath.Separator)) {
				delete(m, path)
			}
		}
	}
	if err = r.save(m); err != nil {
		return "", err
	}
	return backup, r.reconcile(ctx)
}

// adopt writes the node copy at dir into the catalog.
func (r *Reconciler) adopt(ctx context.Context, s data.Skill, dir string) error {
	node, err := readDir(dir)
	if err != nil {
		return err
	}
	if s.Source == "system" {
		fork, err := Fork(ctx, r.Store, s.ID, "")
		if err != nil {
			return err
		}
		s = fork
	}
	s.Description, s.BodyMD, s.Files = node.Description, node.BodyMD, node.Files
	if s.Description == "" {
		return fmt.Errorf("%w: el SKILL.md del nodo no tiene descripción", data.ErrInvalid)
	}
	_, err = Save(ctx, r.Store, s)
	return err
}

// readDir reads a skill folder as the catalog stores it.
func readDir(dir string) (data.Skill, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return data.Skill{}, err
	}
	s, err := parseSkillMD(string(raw))
	if err != nil {
		return s, fmt.Errorf("%w: %v", data.ErrInvalid, err)
	}
	size := len(raw)
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return s, err
	}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			if rel != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == "SKILL.md" || strings.HasPrefix(d.Name(), ".") || !d.Type().IsRegular() {
			return nil
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if size += len(content); size > maxAdopt {
			return fmt.Errorf("%w: la skill del nodo ocupa más de 1 MB", data.ErrInvalid)
		}
		s.Files[filepath.ToSlash(rel)] = string(content)
		return nil
	})
	return s, err
}

// moveAside moves a skill folder, or the link that stands for it, to backup.
// A link is recorded with its target and removed; its target is left alone.
func moveAside(path, backup string) error {
	if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
		return err
	}
	if target, err := os.Readlink(path); err == nil {
		if err = os.WriteFile(backup+".link", []byte(target+"\n"), 0o644); err != nil {
			return err
		}
		return os.Remove(path)
	}
	err := os.Rename(path, backup)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
