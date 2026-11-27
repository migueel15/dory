package codex

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
	"github.com/migueel15/dory/internal/harness/procpool"
	"github.com/migueel15/dory/internal/update"
)

// Codex runs turns over "codex app-server" (codex_app.go); a later turn
// resumes the same thread. codexParser maps the items of "codex exec --json",
// which app-server items are converted to.
type Codex struct {
	pool    *procpool.Pool
	cache   *kit.SummaryCache
	history *kit.UsageCache
	Bin     string
	// Account tells one extra account of this harness from the default one;
	// Home is its CODEX_HOME and Label its display name.
	Account, Home, Label string
}

func (c Codex) ID() string { return harness.AccountID("codex", c.Account) }

// Environ is the environment of every process run for this account.
func (c Codex) Environ() []string { return kit.AccountEnv(c.ConfigEnv(), c.Home) }

func (c Codex) bin() string {
	if c.Bin != "" {
		return c.Bin
	}
	return kit.ResolveBin("codex")
}

func (c Codex) Detect(ctx context.Context) harness.Status {
	version, ok := kit.Detect(ctx, c.bin())
	efforts := []string{"low", "medium", "high", "xhigh"}
	status := harness.Status{ID: c.ID(), Driver: "codex", Name: kit.AccountName("Codex", c.Label), Home: c.Home, Installed: ok, Version: version,
		Models: []harness.Model{{Slug: "gpt-6.2-sol", Name: "GPT-6.2-Sol", Efforts: efforts}},
	}
	if ok {
		if models := codexModels(ctx, c.bin(), c.Environ()); len(models) > 0 {
			status.Models, status.Catalog = models, true
		}
	}
	return status
}

func itemMessage(run string, it codexAppItem, done bool) *harness.Message {
	m := &harness.Message{ID: run + ":" + it.ID, RunID: run, CreatedAt: kit.Now()}
	status := "running"
	if done {
		status = "completed"
	}
	switch it.Type {
	case "commandExecution":
		m.Role, m.Title, m.Detail, m.Text = "tool", "", kit.Short(it.Command, 300), kit.Clip(it.AggregatedOutput, 4000)
		m.Tool = &harness.ToolCall{Kind: "command", Name: "command_execution", Command: it.Command, ExitCode: it.ExitCode}
		if it.ExitCode != nil && *it.ExitCode != 0 || it.Status == "failed" {
			status = "failed"
		}
	case "fileChange":
		var paths []string
		for _, c := range it.Changes {
			paths = append(paths, c.Kind.Type+" "+c.Path)
		}
		m.Role, m.Title, m.Detail = "tool", "", strings.Join(paths, "\n")
		m.Tool = &harness.ToolCall{Kind: "edit", Name: "file_change"}
		for _, c := range it.Changes {
			m.Tool.Files = append(m.Tool.Files, harness.FileEdit{Path: c.Path, Change: c.Kind.Type})
		}
	case "mcpToolCall":
		m.Role, m.Title = "tool", it.Server+"."+it.Tool
		m.Tool = &harness.ToolCall{Kind: "mcp", Name: it.Tool, Server: it.Server, Tool: it.Tool}
		if len(it.Arguments) > 0 && string(it.Arguments) != "null" {
			m.Tool.Input = kit.Clip(string(it.Arguments), 4000)
		}
	case "webSearch":
		m.Role, m.Title, m.Detail = "tool", "", it.Query
		m.Tool = &harness.ToolCall{Kind: "web", Name: "web_search", Query: it.Query}
	default:
		return nil
	}
	m.Status = status
	return m
}

// stripCodexMemoryCitations keeps a possible opening tag out of streaming
// snapshots until more text arrives and resolves whether it starts a block.
func (c Codex) Info() harness.Info {
	return harness.Info{ID: c.ID(), Driver: "codex", Name: "Codex", Order: 1,
		Capabilities: harness.Capabilities{Resume: true, MidTurnRequests: true, UsageReport: true, SystemPrompt: true, Effort: true, OfflineHistory: true}}
}

func (c Codex) WithPool(p *procpool.Pool) harness.Driver {
	c.pool = p
	c.cache = &kit.SummaryCache{}
	c.history = &kit.UsageCache{}
	return c
}
func (c Codex) WithAccount(a harness.Account) harness.Driver {
	c.Account, c.Home, c.Label = a.Slug, a.Home, a.Name
	return c
}

func (Codex) ConfigEnv() string              { return "CODEX_HOME" }
func (Codex) DefaultHome(home string) string { return filepath.Join(home, ".codex") }
func (c Codex) AccountHome() string          { return c.Home }

func (Codex) ProcessNames() []string                   { return []string{"codex"} }
func (Codex) LatestVersion(ctx context.Context) string { return update.NPMLatest(ctx, "@openai/codex") }
func (c Codex) Commands(p harness.Paths, dir string) (commands, skills []harness.Entry) {
	home := p.Home
	commands = kit.MarkdownEntries(filepath.Join(kit.AccountDir(p, c), "prompts"), "/prompts:")
	for _, root := range []string{filepath.Join(kit.AccountDir(p, c), "skills"), filepath.Join(home, ".agents", "skills"), filepath.Join(dir, ".codex", "skills"), filepath.Join(dir, ".agents", "skills")} {
		skills = append(skills, kit.SkillEntries(root, "$")...)
	}
	return
}

func (c Codex) SkillDir(p harness.Paths) harness.SkillLocation {
	return harness.SkillLocation{Dir: filepath.Join(kit.AccountDir(p, c), "skills")}
}
