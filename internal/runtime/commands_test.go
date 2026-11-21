package runtime

import (
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
	commands, skills := Commands(home, dir, "claude")
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
	if c, _ := Commands(home, dir, "codex"); len(c) != 1 || c[0].Name != "clear" {
		t.Fatalf("codex = %+v", c)
	}
}
