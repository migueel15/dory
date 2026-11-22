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
	s.ID, s.SystemVersion = SystemPrefix+s.Name, s.Version
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

// SeedSystem writes the system skills of this binary into the catalog. It
// keeps what the mesh decided about them: whether they are on, their targets
// and their forks. It writes a skill when it is missing, when the catalog has
// an older shipped version, or when the catalog has this same version with
// other content or name: a binary that predates system skills edited it, or
// a user skill took its name.
func SeedSystem(ctx context.Context, store *data.Store) error {
	shipped, err := System()
	if err != nil {
		return err
	}
	for _, s := range shipped {
		cur, err := store.Skills().Get(ctx, s.ID)
		missing := errors.Is(err, data.ErrNotFound)
		if err != nil && !missing {
			return err
		}
		if !missing && cur.Source == "system" && (cur.SystemVersion > s.Version ||
			cur.SystemVersion == s.Version && cur.Name == s.Name && cur.Description == s.Description && cur.BodyMD == s.BodyMD && maps(cur.Files, s.Files)) {
			continue // Up to date, or a newer binary owns it.
		}
		forkOn, err := adoptName(ctx, store, s)
		if err != nil {
			return err
		}
		next := s
		if !missing {
			next = cur
			next.Name, next.Description, next.BodyMD, next.Files, next.Source, next.SystemVersion = s.Name, s.Description, s.BodyMD, s.Files, "system", s.Version
			next.Version = max(cur.Version+1, s.Version)
		}
		if forkOn && next.DisabledAt == nil {
			now := time.Now().UTC().Format(time.RFC3339Nano)
			next.DisabledAt = &now
		}
		if _, err = store.Skills().Put(ctx, next); err != nil {
			return err
		}
	}
	return nil
}

// adoptName turns a user skill that holds the name of a system skill into a
// fork of it, so the system skill gets its name on every node. It says
// whether that fork is on, in which case the system skill must stay off.
func adoptName(ctx context.Context, store *data.Store, s data.Skill) (bool, error) {
	holders, err := store.Skills().List(ctx, data.Filter{Fields: map[string]any{"name": s.Name}, Limit: 10, IncludeArchived: true})
	if err != nil {
		return false, err
	}
	on := false
	for _, h := range holders {
		if h.ID == s.ID {
			continue
		}
		h.Name, h.ForkedFrom = s.Name+"-custom", &s.ID
		if _, err = store.Skills().Put(ctx, h); err != nil {
			return false, err
		}
		on = on || h.ArchivedAt == nil && h.DisabledAt == nil
	}
	return on, nil
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
