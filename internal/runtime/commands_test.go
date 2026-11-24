package runtime

import (
	"github.com/migueel15/dory/internal/harness/claude"
	"github.com/migueel15/dory/internal/harness/codex"
	"os"
	"path/filepath"
	"testing"
)

func TestCommandsClaude(t *testing.T) {
	home, dir := t.TempDir(), t.TempDir()
	write := func(path, body string) {
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte(body), 0o644)
	}
	write(filepath.Join(dir, ".claude", "commands", "x.md"), "---\ndescription: \"Hace x\"\n---\nbody")
	write(filepath.Join(dir, ".claude", "commands", "clear.md"), "harness clear")
	write(filepath.Join(home, ".claude", "skills", "s", "SKILL.md"), "---\nname: s\ndescription: Skill s\n---\n")
	write(filepath.Join(home, ".claude", "commands", "s.md"), "dup of skill")
	commands, skills := (claude.Claude{}).Commands(Paths{Home: home}, dir)
	commands, skills = composerCommands(commands, skills)
	got := map[string]Entry{}
	for _, c := range commands {
		got[c.Name] = c
	}
	if got["x"].Description != "Hace x" || got["x"].Insert != "/x" {
		t.Fatalf("x = %+v", got["x"])
	}
	if got["clear"].Insert != ClearCommand || got["clear"].Description == "harness clear" {
		t.Fatalf("clear = %+v", got["clear"])
	}
	if _, ok := got["s"]; ok {
		t.Fatal("command repeating a skill kept")
	}
	if _, ok := got["compact"]; !ok {
		t.Fatal("builtin compact missing")
	}
	if len(skills) != 1 || skills[0].Insert != "/s" {
		t.Fatalf("skills = %+v", skills)
	}
	if c, _ := composerCommands((codex.Codex{}).Commands(Paths{Home: home}, dir)); len(c) != 1 || c[0].Name != "clear" {
		t.Fatalf("codex = %+v", c)
	}
}

func TestPathsFollowDefaultAccount(t *testing.T) {
	custom := t.TempDir()
	p := Paths{Home: "/h", Config: func(d string) string {
		if d == "claude" {
			return custom
		}
		return ""
	}}
	if p.Dir("claude") != custom || p.Dir("codex") != filepath.Join("/h", ".codex") {
		t.Fatalf("dirs = %s, %s", p.Dir("claude"), p.Dir("codex"))
	}
}
