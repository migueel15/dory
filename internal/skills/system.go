package skills

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/data"
)

// System skills ship inside the binary. Each node seeds them into the
// replicated catalog when it starts; a node never replaces a newer version,
// so a mesh with mixed binaries converges on the newest one. Their frontmatter
// carries a version that must grow whenever the skill changes.
//
//go:embed system
var systemFS embed.FS

// SystemPrefix starts the id of every system skill, so all nodes agree on it.
const SystemPrefix = "system-"

// System returns the skills this binary ships.
func System() ([]data.Skill, error) {
	dirs, err := fs.ReadDir(systemFS, "system")
	if err != nil {
		return nil, err
	}
	var out []data.Skill
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		s, err := readSystem(path.Join("system", d.Name()))
		if err != nil {
			return nil, fmt.Errorf("system skill %s: %w", d.Name(), err)
		}
		out = append(out, s)
	}
	return out, nil
}

func readSystem(dir string) (data.Skill, error) {
	raw, err := fs.ReadFile(systemFS, path.Join(dir, "SKILL.md"))
	if err != nil {
		return data.Skill{}, err
	}
	front, body, ok := strings.Cut(strings.TrimPrefix(string(raw), "---\n"), "\n---\n")
	if !ok {
		return data.Skill{}, errors.New("missing frontmatter")
	}
	s := data.Skill{BodyMD: strings.TrimSpace(body), Source: "system", Files: map[string]string{}}
	for _, line := range strings.Split(front, "\n") {
		key, value, _ := strings.Cut(line, ":")
		value = strings.TrimSpace(value)
		if unq, err := strconv.Unquote(value); err == nil {
			value = unq
		}
		switch strings.TrimSpace(key) {
		case "name":
			s.Name = value
		case "description":
			s.Description = value
		case "version":
			if s.Version, err = strconv.ParseInt(value, 10, 64); err != nil {
				return s, fmt.Errorf("bad version %q", value)
			}
		}
	}
	if s.Name != path.Base(dir) || s.Version < 1 || s.Description == "" {
		return s, errors.New("frontmatter needs name (the folder), description and version")
	}
	s.ID = SystemPrefix + s.Name
	err = fs.WalkDir(systemFS, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Base(p) == "SKILL.md" && path.Dir(p) == dir {
			return err
		}
		content, err := fs.ReadFile(systemFS, p)
		s.Files[strings.TrimPrefix(p, dir+"/")] = string(content)
		return err
	})
	return s, err
}

// SeedSystem writes the system skills of this binary into the catalog when
// they are missing or older there. It keeps what the mesh decided about them:
// whether they are on, their targets and their forks.
func SeedSystem(ctx context.Context, store *data.Store) error {
	shipped, err := System()
	if err != nil {
		return err
	}
	for _, s := range shipped {
		cur, err := store.Skills().Get(ctx, s.ID)
		if errors.Is(err, data.ErrNotFound) {
			if _, err = store.Skills().Put(ctx, s); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if cur.Version >= s.Version && cur.Source == "system" {
			continue
		}
		cur.Name, cur.Description, cur.BodyMD, cur.Files, cur.Version, cur.Source = s.Name, s.Description, s.BodyMD, s.Files, s.Version, "system"
		if _, err = store.Skills().Put(ctx, cur); err != nil {
			return err
		}
	}
	return nil
}

// SetEnabled turns a skill on or off on every node. A system skill and its
// forks never run together: enabling a fork turns its system skill off, and
// a system skill can only be enabled once its forks are off.
func SetEnabled(ctx context.Context, store *data.Store, id string, enabled bool) (data.Skill, error) {
	s, err := store.Skills().Get(ctx, id)
	if err != nil {
		return s, err
	}
	if enabled == (s.DisabledAt == nil) {
		return s, nil
	}
	if !enabled {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		s.DisabledAt = &now
		return store.Skills().Put(ctx, s)
	}
	if s.Source == "system" {
		forks, err := store.Skills().List(ctx, data.Filter{Fields: map[string]any{"forked_from": s.ID}, Limit: 1000})
		if err != nil {
			return s, err
		}
		for _, f := range forks {
			if f.DisabledAt == nil {
				return s, fmt.Errorf("%w: desactiva antes el fork %s", data.ErrInvalid, f.Name)
			}
		}
	}
	if s.ForkedFrom != nil {
		if _, err := SetEnabled(ctx, store, *s.ForkedFrom, false); err != nil && !errors.Is(err, data.ErrNotFound) {
			return s, err
		}
	}
	s.DisabledAt = nil
	return store.Skills().Put(ctx, s)
}

// Fork copies a system skill into an editable one and turns the original off.
func Fork(ctx context.Context, store *data.Store, id, name string) (data.Skill, error) {
	orig, err := store.Skills().Get(ctx, id)
	if err != nil {
		return orig, err
	}
	if orig.Source != "system" {
		return orig, fmt.Errorf("%w: solo se puede hacer fork de una skill del sistema", data.ErrInvalid)
	}
	if name == "" {
		name = orig.Name + "-custom"
	}
	fork, err := Save(ctx, store, data.Skill{Name: name, Description: orig.Description, BodyMD: orig.BodyMD, Files: orig.Files, Targets: orig.Targets, ForkedFrom: &orig.ID})
	if err != nil {
		return fork, err
	}
	if _, err = SetEnabled(ctx, store, orig.ID, false); err != nil {
		return fork, err
	}
	return fork, nil
}
