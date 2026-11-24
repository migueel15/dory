package kit

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/migueel15/dory/internal/harness"
)

func MarkdownEntries(root, prefix string) []harness.Entry {
	var out []harness.Entry
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		name := strings.ReplaceAll(strings.TrimSuffix(rel, ".md"), string(filepath.Separator), ":")
		out = append(out, harness.Entry{Name: name, Description: describe(path), Insert: prefix + name})
		return nil
	})
	return out
}

// skillEntries finds every SKILL.md under root, following the symbolic links
// the user and Dory use to share skills between harnesses.
func SkillEntries(root, prefix string) []harness.Entry {
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

func describe(path string) string { return Short(frontmatter(path, "description"), 160) }

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

func UniqueEntries(list []harness.Entry) []harness.Entry {
	seen := map[string]bool{}
	out := []harness.Entry{}
	for _, e := range list {
		if !seen[e.Insert] {
			seen[e.Insert] = true
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
