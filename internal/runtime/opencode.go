package runtime

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/cmdexec"
	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/procpool"
	"github.com/migueel15/dory/internal/update"
)

// OpenCode runs each turn on its own "opencode serve" (opencode_server.go);
// a later turn continues the same session.
type OpenCode struct {
	Bin  string
	pool *procpool.Pool
}

func (OpenCode) ID() string { return "opencode" }

func (o OpenCode) bin() string {
	if o.Bin != "" {
		return o.Bin
	}
	return resolveBin("opencode")
}

func (o OpenCode) Detect(ctx context.Context) Status {
	version, ok := detect(ctx, o.bin())
	st := Status{ID: "opencode", Name: "OpenCode", Installed: ok, Version: version}
	if !ok {
		return st
	}
	// Models depend on the providers configured in OpenCode: ask it.
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, _ := cmdexec.CommandContext(ctx, o.bin(), "models").Output()
	st.Models = append(st.Models, Model{Slug: "", Name: "Por defecto"})
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			st.Models = append(st.Models, Model{Slug: line, Name: line})
		}
	}
	return st
}

func (o OpenCode) Info() harness.Info {
	return harness.Info{ID: o.ID(), Driver: o.ID(), Name: "OpenCode", Order: 2,
		Capabilities: harness.Capabilities{Resume: true, MidTurnRequests: true, SystemPrompt: true}}
}

func (o OpenCode) WithPool(p *procpool.Pool) harness.Driver { o.pool = p; return o }

func (OpenCode) ProcessNames() []string { return []string{"opencode"} }
func (OpenCode) LatestVersion(ctx context.Context) string {
	return update.NPMLatest(ctx, "opencode-ai")
}
func (o OpenCode) IsControlProcess(command string) bool {
	return strings.Contains(command, o.ID()+" serve --hostname 127.0.0.1 --port 0")
}

func (o OpenCode) Commands(p Paths, dir string) (commands, skills []Entry) {
	home := p.Home
	for _, root := range []string{filepath.Join(home, ".config", "opencode", "command"), filepath.Join(dir, ".opencode", "command")} {
		commands = append(commands, markdownEntries(root, "/")...)
	}
	for _, root := range []string{o.SkillDir(p).Dir, filepath.Join(p.Dir((Claude{}).ID()), "skills"), filepath.Join(dir, ".opencode", "skills")} {
		skills = append(skills, skillEntries(root, "$")...)
	}
	return
}

func (OpenCode) SkillDir(p Paths) harness.SkillLocation {
	return harness.SkillLocation{Dir: filepath.Join(p.Home, ".config", "opencode", "skills")}
}
