package runtime

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/migueel15/dory/internal/harness"
)

type Entry = harness.Entry

// Commands lists the slash commands and skills available in dir.
func (m *Manager) Commands(p Paths, dir, provider string) (commands, skills []Entry) {
	if d, ok := m.registry.Get(provider); ok && d.Commands != nil {
		commands, skills = d.Commands.Commands(p, dir)
	}
	return composerCommands(commands, skills)
}

func composerCommands(commands, skills []Entry) ([]Entry, []Entry) {
	// Dory runs /clear itself for every harness, so the harness's own is hidden.
	commands = append([]Entry{{Name: "clear", Description: "Empieza una sesión nueva del agente; los mensajes siguen visibles", Insert: ClearCommand}}, commands...)
	var kept []Entry
	names := map[string]bool{}
	for _, s := range skills {
		names[s.Name] = true
	}
	for i, c := range commands {
		// A command named like a skill repeats it, as T3 Code does.
		if (i > 0 && c.Name == "clear") || names[c.Name] {
			continue
		}
		kept = append(kept, c)
	}
	return unique(kept), unique(skills)
}

// markdownEntries reads commands kept as one markdown file each; folders
// namespace them as a:b.
func markdownEntries(root, prefix string) []Entry {
	var out []Entry
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		name := strings.ReplaceAll(strings.TrimSuffix(rel, ".md"), string(filepath.Separator), ":")
		out = append(out, Entry{Name: name, Description: describe(path), Insert: prefix + name})
		return nil
	})
	return out
}

// skillEntries finds every SKILL.md under root, following the symbolic links
// the user and Dory use to share skills between harnesses.
func skillEntries(root, prefix string) []Entry {
	var out []Entry
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > 3 {
			return
		}
		items, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, it := range items {
			path := filepath.Join(dir, it.Name())
			st, err := os.Stat(path) // follows links
			if err != nil || !st.IsDir() || strings.HasPrefix(it.Name(), ".") {
				continue
			}
			if skill := filepath.Join(path, "SKILL.md"); fileExists(skill) {
				name := frontmatter(skill, "name")
				if name == "" {
					name = it.Name()
				}
				out = append(out, Entry{Name: name, Description: describe(skill), Insert: prefix + name})
				continue
			}
			walk(path, depth+1)
		}
	}
	walk(root, 0)
	return out
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func describe(path string) string { return short(frontmatter(path, "description"), 160) }

// frontmatter reads one field of a markdown file's YAML header.
func frontmatter(path, field string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	if !scan.Scan() || strings.TrimSpace(scan.Text()) != "---" {
		return ""
	}
	for scan.Scan() {
		line := scan.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		if v, ok := strings.CutPrefix(line, field+":"); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

func unique(list []Entry) []Entry {
	seen := map[string]bool{}
	out := []Entry{}
	for _, e := range list {
		if !seen[e.Insert] {
			seen[e.Insert] = true
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (m *Manager) SkillDirs(home string) map[string]harness.SkillLocation {
	out := map[string]harness.SkillLocation{}
	p := m.Paths(home)
	for _, d := range m.registry.All() {
		info := d.Info()
		if info.ID == info.Driver && d.Commands != nil {
			out[info.Driver] = d.Commands.SkillDir(p)
		}
	}
	return out
}
