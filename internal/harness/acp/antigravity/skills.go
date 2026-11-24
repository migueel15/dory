package antigravity

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/migueel15/dory/internal/harness"
)

func skillDir(p harness.Paths) harness.SkillLocation {
	return harness.SkillLocation{Dir: filepath.Join(p.Home, ".gemini", "config", "skills"), ManagedSubdir: "antigravity-skills"}
}

func commands(p harness.Paths, dir string) (commands, skills []harness.Entry) {
	for _, root := range []string{skillDir(p).Dir, filepath.Join(p.Home, ".gemini", "antigravity-cli", "skills"), filepath.Join(dir, ".gemini", "skills"), filepath.Join(dir, ".agents", "skills")} {
		skills = append(skills, skillEntries(root, "/")...)
	}
	return
}

// skillEntries finds every SKILL.md under root, following the symbolic links
// the user and Dory use to share skills between harnesses.
func skillEntries(root, prefix string) []harness.Entry {
	var out []harness.Entry
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
				out = append(out, harness.Entry{Name: name, Description: describe(skill), Insert: prefix + name})
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

func describe(path string) string {
	s, _, _ := strings.Cut(strings.TrimSpace(frontmatter(path, "description")), "\n")
	if r := []rune(s); len(r) > 160 {
		return string(r[:159]) + "…"
	}
	return s
}

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
