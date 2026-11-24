package runtime

import (
	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
	"github.com/migueel15/dory/internal/i18n"
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
	commands = append([]Entry{{Name: "clear", Description: i18n.Text("command.clear"), Insert: ClearCommand}}, commands...)
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
	return kit.UniqueEntries(kept), kit.UniqueEntries(skills)
}

// markdownEntries reads commands kept as one markdown file each; folders
// namespace them as a:b.

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
