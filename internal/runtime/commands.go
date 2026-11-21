package runtime

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is a slash command or a skill a harness offers in a folder.
type Entry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Insert is what the composer writes: how the harness invokes it.
	Insert string `json:"insert"`
}

// Commands lists the slash commands and skills of a provider for a thread
// working in dir, from the user's and the project's configuration.
func Commands(home, dir, provider string) (commands, skills []Entry) {
	switch provider {
	case "claude":
		for _, root := range []string{filepath.Join(home, ".claude", "commands"), filepath.Join(dir, ".claude", "commands")} {
			commands = append(commands, markdownEntries(root, "/")...)
		}
		for _, root := range []string{filepath.Join(home, ".claude", "skills"), filepath.Join(dir, ".claude", "skills")} {
			// Claude Code runs a skill as a slash command.
			skills = append(skills, skillEntries(root, "/")...)
		}
	case "codex":
		commands = markdownEntries(filepath.Join(home, ".codex", "prompts"), "/prompts:")
		for _, root := range []string{filepath.Join(home, ".codex", "skills"), filepath.Join(home, ".agents", "skills"), filepath.Join(dir, ".codex", "skills"), filepath.Join(dir, ".agents", "skills")} {
			skills = append(skills, skillEntries(root, "$")...)
		}
	case antigravityID:
		for _, root := range []string{filepath.Join(home, ".gemini", "config", "skills"), filepath.Join(home, ".gemini", "antigravity-cli", "skills"), filepath.Join(dir, ".gemini", "skills"), filepath.Join(dir, ".agents", "skills")} {
			skills = append(skills, skillEntries(root, "/")...)
		}
	case "opencode":
		for _, root := range []string{filepath.Join(home, ".config", "opencode", "command"), filepath.Join(dir, ".opencode", "command")} {
			commands = append(commands, markdownEntries(root, "/")...)
		}
		for _, root := range []string{filepath.Join(home, ".config", "opencode", "skills"), filepath.Join(home, ".claude", "skills"), filepath.Join(dir, ".opencode", "skills")} {
			skills = append(skills, skillEntries(root, "$")...)
		}
	}
	// Dory runs /clear itself for every harness, so the harness's own is hidden.
	commands = append(builtins[provider], commands...)
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

// builtins are the harness commands that work in a headless session.
var builtins = map[string][]Entry{
	"claude": {
		{Name: "compact", Description: "Resume la conversación para liberar contexto", Insert: "/compact"},
		{Name: "context", Description: "Muestra el uso de la ventana de contexto", Insert: "/context"},
		{Name: "cost", Description: "Muestra el coste de la sesión", Insert: "/cost"},
		{Name: "init", Description: "Crea CLAUDE.md con la documentación del repositorio", Insert: "/init"},
		{Name: "review", Description: "Revisa un pull request", Insert: "/review"},
		{Name: "security-review", Description: "Revisión de seguridad de los cambios pendientes", Insert: "/security-review"},
		{Name: "pr-comments", Description: "Trae los comentarios de un pull request", Insert: "/pr-comments"},
	},
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
