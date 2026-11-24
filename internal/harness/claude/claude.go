package claude

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/migueel15/dory/internal/i18n"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
	"github.com/migueel15/dory/internal/harness/procpool"
	"github.com/migueel15/dory/internal/update"
)

// Claude runs Claude Code headless with its stream-json output.
type Claude struct {
	pool    *procpool.Pool
	cache   *kit.SummaryCache
	history *kit.UsageCache
	Bin     string
	// Account tells one extra account of this harness from the default one;
	// Home is its CLAUDE_CONFIG_DIR and Label its display name.
	Account, Home, Label string
}

func (c Claude) ID() string { return harness.AccountID("claude", c.Account) }

// Environ is the environment of every process run for this account.
func (c Claude) Environ() []string { return kit.AccountEnv(c.ConfigEnv(), c.Home) }

func (c Claude) bin() string {
	if c.Bin != "" {
		return c.Bin
	}
	return kit.ResolveBin("claude")
}

func (c Claude) Detect(ctx context.Context) harness.Status {
	version, ok := kit.Detect(ctx, c.bin())
	status := harness.Status{ID: c.ID(), Driver: "claude", Name: kit.AccountName("Claude Code", c.Label), Home: c.Home, Installed: ok, Version: version,
		Models: []harness.Model{
			{Slug: "opus", Name: "Opus", Efforts: []string{"low", "medium", "high", "xhigh", "max"}},
			{Slug: "sonnet", Name: "Sonnet", Efforts: []string{"low", "medium", "high", "xhigh", "max"}},
			{Slug: "haiku", Name: "Haiku"},
		},
	}
	if ok {
		if models := claudeModels(ctx, c.bin(), c.Environ()); len(models) > 0 {
			status.Models, status.Catalog = models, true
		}
	}
	return status
}

// Start runs a turn with stream-json on both ends. The process asks Dory over
// stdio before using a tool, so AskUserQuestion pauses the turn until the
// user answers it. The process stays open for the next turn of the thread
// (see live.go), so what the agent left running in the background survives.
func (c Claude) Start(ctx context.Context, t harness.Turn) (harness.Session, error) {
	args := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose",
		"--include-partial-messages", "--permission-prompt-tool", "stdio"}
	if t.Model != "" {
		args = append(args, "--model", t.Model)
	}
	if t.Effort != "" {
		args = append(args, "--effort", t.Effort)
	}
	if t.Session != "" {
		args = append(args, "--resume", t.Session)
	}
	if t.FullAccess {
		args = append(args, "--dangerously-skip-permissions")
	} else {
		args = append(args, "--permission-mode", "acceptEdits")
	}
	if t.MCPURL != "" {
		cfg, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"dory": map[string]any{
			"type": "http", "url": t.MCPURL, "headers": map[string]string{"Authorization": "Bearer ${DORY_MCP_TOKEN}"},
		}}})
		args = append(args, "--mcp-config", string(cfg))
	}
	if t.Instructions != "" {
		args = append(args, "--append-system-prompt", t.Instructions)
	}
	key := strings.Join([]string{t.Model, t.Effort, fmt.Sprint(t.FullAccess), t.Dir, c.Home, t.MCPURL, t.Instructions}, "\x00")
	p := kit.ReuseStdio(c.pool, t.ThreadID, c.ID(), key)
	if p == nil {
		cmd := kit.ProviderCommand(context.Background(), t.ThreadID, c.bin(), args...)
		cmd.Dir = t.Dir
		// ask_user waits for the user, so an MCP call may take long (ms).
		cmd.Env = append(kit.ThreadEnv(c.Environ(), t.ThreadID), "MCP_TOOL_TIMEOUT=86400000")
		if t.MCPURL != "" {
			cmd.Env = append(cmd.Env, "DORY_MCP_TOKEN="+t.MCPToken)
		}
		var err error
		if p, err = kit.OpenStdio(c.pool, cmd, t.ThreadID, c.ID(), key); err != nil {
			return nil, err
		}
	}
	s := newClaudeSession(t.RunID, p.Input, t.FullAccess)
	turn := p.Begin(t.ThreadID, s.handle)
	s.events = turn.Events
	// A turn cancellation is a protocol request, not a signal to what the
	// agent started.
	p.Watch(ctx, t.ThreadID, c.ID(), turn, func() {
		_ = s.write(map[string]any{"type": "control_request", "request_id": rand.Text(), "request": map[string]any{"subtype": "interrupt"}})
	})
	if err := s.write(map[string]any{"type": "user", "parent_tool_use_id": nil,
		"message": map[string]any{"role": "user", "content": t.Text}}); err != nil {
		p.Abort(turn)
		return nil, err
	}
	return s, nil
}

// claudeSession speaks Claude Code's control protocol on stdin while its
// parser follows stdout.
type claudeSession struct {
	parser     *claudeParser
	events     <-chan harness.Event
	fullAccess bool

	mu      sync.Mutex
	in      io.WriteCloser
	pending map[string]claudeAsk
}

// claudeAsk is an AskUserQuestion call waiting for the user.
type claudeAsk struct {
	input     map[string]any
	questions []harness.Question
}

func newClaudeSession(run string, in io.WriteCloser, fullAccess bool) *claudeSession {
	return &claudeSession{parser: &claudeParser{run: run, texts: map[string]*kit.Text{}, tools: map[string]*harness.Message{}},
		in: in, fullAccess: fullAccess, pending: map[string]claudeAsk{}}
}

func (s *claudeSession) Events() <-chan harness.Event { return s.events }

// Respond lets the pending AskUserQuestion run with the answers, keyed by
// question text; several choices go joined by ", " as Claude expects.
func (s *claudeSession) Respond(id string, answers [][]string) error {
	s.mu.Lock()
	ask, ok := s.pending[id]
	delete(s.pending, id)
	s.mu.Unlock()
	if !ok {
		return harness.ErrNoRequest
	}
	byQuestion := map[string]string{}
	for i, q := range ask.questions {
		if i < len(answers) {
			byQuestion[q.Question] = strings.Join(answers[i], ", ")
		}
	}
	input := map[string]any{}
	for k, v := range ask.input {
		input[k] = v
	}
	input["answers"] = byQuestion
	return s.allow(id, input)
}

func (s *claudeSession) handle(line []byte, emit func(harness.Event)) {
	var l claudeLine
	if json.Unmarshal(line, &l) != nil {
		return
	}
	switch l.Type {
	case "control_request":
		s.control(l, emit)
	case "control_cancel_request":
		s.mu.Lock()
		delete(s.pending, l.RequestID)
		s.mu.Unlock()
	default:
		s.parser.parse(line, emit)
	}
}

// control answers a permission request of the CLI. AskUserQuestion becomes
// a pending request; other tools follow the access mode of the thread.
func (s *claudeSession) control(l claudeLine, emit func(harness.Event)) {
	r := l.Request
	if r.Subtype != "can_use_tool" {
		s.write(map[string]any{"type": "control_response", "response": map[string]any{
			"subtype": "error", "request_id": l.RequestID, "error": "unsupported request: " + r.Subtype}})
		return
	}
	var input map[string]any
	_ = json.Unmarshal(r.Input, &input)
	if r.ToolName == "AskUserQuestion" {
		qs := kit.ParseQuestions(r.Input)
		s.mu.Lock()
		s.pending[l.RequestID] = claudeAsk{input: input, questions: qs}
		s.mu.Unlock()
		emit(harness.RequestOpened{Request: harness.Request{ID: l.RequestID, Kind: "question", MessageID: r.ToolUseID, Questions: qs, CreatedAt: kit.Now()}})
		return
	}
	if s.fullAccess {
		s.allow(l.RequestID, input)
		return
	}
	// Approval prompts are not in Dory yet: without full access, tools that
	// need one are refused as before.
	s.write(map[string]any{"type": "control_response", "response": map[string]any{
		"subtype": "success", "request_id": l.RequestID,
		"response": map[string]any{"behavior": "deny", "message": "This thread does not allow " + r.ToolName + " without approval."}}})
}

func (s *claudeSession) allow(id string, input map[string]any) error {
	return s.write(map[string]any{"type": "control_response", "response": map[string]any{
		"subtype": "success", "request_id": id,
		"response": map[string]any{"behavior": "allow", "updatedInput": input}}})
}

func (s *claudeSession) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.in.Write(append(b, '\n'))
	return err
}

// claudeParser follows one turn. Streamed blocks are keyed by message id and
// block index, which the final assistant event repeats.
type claudeParser struct {
	run     string
	current string
	texts   map[string]*kit.Text
	tools   map[string]*harness.Message
	// blocks counts the content blocks seen of each message. Claude Code
	// writes each block of a message on its own line, so a block's place in
	// its line is always 0; its place in the message is this count, the same
	// index the streamed partial blocks use.
	blocks    map[string]int
	usageID   string
	usage     harness.StepUsage
	usageSeen map[string]bool
}

type claudeBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

type claudeTokens struct {
	Input      int64 `json:"input_tokens"`
	Output     int64 `json:"output_tokens"`
	CacheRead  int64 `json:"cache_read_input_tokens"`
	CacheWrite int64 `json:"cache_creation_input_tokens"`
}

type claudeLine struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	SessionID string `json:"session_id"`
	Model     string `json:"model"`
	Effort    string `json:"effort"`
	IsError   bool   `json:"is_error"`
	Result    string `json:"result"`
	RequestID string `json:"request_id"`
	Request   struct {
		Subtype   string          `json:"subtype"`
		ToolName  string          `json:"tool_name"`
		Input     json.RawMessage `json:"input"`
		ToolUseID string          `json:"tool_use_id"`
	} `json:"request"`
	Event struct {
		Type    string `json:"type"`
		Index   int    `json:"index"`
		Message struct {
			ID    string       `json:"id"`
			Model string       `json:"model"`
			Usage claudeTokens `json:"usage"`
		} `json:"message"`
		Usage        claudeTokens `json:"usage"`
		ContentBlock claudeBlock  `json:"content_block"`
		Delta        struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
		} `json:"delta"`
	} `json:"event"`
	Message struct {
		ID      string        `json:"id"`
		Model   string        `json:"model"`
		Usage   claudeTokens  `json:"usage"`
		Content []claudeBlock `json:"content"`
	} `json:"message"`
	// ToolUseResult carries the answers of an AskUserQuestion result.
	ToolUseResult struct {
		Answers map[string]string `json:"answers"`
	} `json:"tool_use_result"`
	ParentToolUseID *string `json:"parent_tool_use_id"`
	RateLimitInfo   struct {
		Unified map[string]struct {
			Utilization float64 `json:"utilization"`
			ResetsAt    int64   `json:"resetsAt"`
		} `json:"unifiedWindows"`
	} `json:"rate_limit_info"`
}

func (p *claudeParser) parse(line []byte, emit func(harness.Event)) {
	var l claudeLine
	if json.Unmarshal(line, &l) != nil {
		return
	}
	switch l.Type {
	case "system":
		if l.Subtype == "init" {
			if l.SessionID != "" {
				emit(harness.SessionBound{ID: l.SessionID})
			}
			if l.Model != "" || l.Effort != "" {
				emit(harness.SettingsReported{Model: l.Model, Effort: l.Effort})
			}
		}
	case "stream_event":
		if l.ParentToolUseID == nil {
			switch l.Event.Type {
			case "message_start":
				p.recordUsage(l.Event.Message.ID, l.Event.Message.Model, l.Event.Message.Usage, emit)
			case "message_delta":
				p.usage.Output = max(p.usage.Output, l.Event.Usage.Output)
			}
			p.partial(l, emit)
		}
	case "assistant":
		if l.ParentToolUseID == nil && l.Message.Model != "" && l.Message.Model != "<synthetic>" {
			emit(harness.SettingsReported{Model: l.Message.Model})
		}
		// A subagent's own work stays inside its card: it only counts its calls.
		if l.ParentToolUseID != nil {
			id, u := l.Message.ID, l.Message.Usage
			if p.usageSeen == nil {
				p.usageSeen = map[string]bool{}
			}
			if id != "" && !p.usageSeen[id] && l.Message.Model != "<synthetic>" && u != (claudeTokens{}) {
				p.usageSeen[id] = true
				emit(harness.StepUsage{Model: l.Message.Model, Input: u.Input, Output: u.Output, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite, Subagent: *l.ParentToolUseID})
			}
			if parent := p.tools[*l.ParentToolUseID]; parent != nil && parent.Tool != nil {
				for _, b := range l.Message.Content {
					if b.Type == "tool_use" {
						parent.Tool.Steps++
					}
				}
				emit(kit.UpdateTool(parent))
			}
			return
		}
		if l.Message.Model != "<synthetic>" && l.Message.ID != "" {
			p.recordUsage(l.Message.ID, l.Message.Model, l.Message.Usage, emit)
		}
		if p.blocks == nil {
			p.blocks = map[string]int{}
		}
		for _, b := range l.Message.Content {
			i := p.blocks[l.Message.ID]
			p.blocks[l.Message.ID]++
			p.block(fmt.Sprintf("%s:%d", l.Message.ID, i), b, emit)
		}
	case "user":
		for _, b := range l.Message.Content {
			if b.Type != "tool_result" {
				continue
			}
			if l.ParentToolUseID == nil {
				p.flushUsage(emit)
			}
			if m := p.tools[b.ToolUseID]; m != nil {
				m.ResultChars = len(kit.ResultText(b.Content))
				m.Status = "completed"
				if b.IsError {
					m.Status = "failed"
				}
				if m.Role == "plan" {
					m.Status = "" // the plan keeps its text; the result is only an ack
				} else if m.Tool != nil && m.Tool.Kind == "question" && len(l.ToolUseResult.Answers) > 0 {
					m.Tool.Questions = withAnswers(m.Tool.Questions, l.ToolUseResult.Answers)
					m.Text = ""
				} else {
					m.Text = kit.Clip(kit.ResultText(b.Content), 4000)
				}
				emit(kit.FinishTool(m))
			}
		}
	case "rate_limit_event":
		var windows []harness.Window
		for kind, w := range l.RateLimitInfo.Unified {
			windows = append(windows, harness.Window{Kind: kind, Utilization: w.Utilization, ResetsAt: time.Unix(w.ResetsAt, 0).UTC()})
		}
		sort.Slice(windows, func(i, j int) bool { return windows[i].Kind < windows[j].Kind })
		if len(windows) > 0 {
			emit(harness.UsageReported{Windows: windows})
		}
	case "result":
		p.flushUsage(emit)
		for _, id := range slices.Sorted(maps.Keys(p.texts)) {
			text := p.texts[id]
			if text.String() != "" {
				emit(text.Done(text.String()))
			}
		}
		e := harness.TurnEnded{Reason: harness.Completed}
		if l.IsError || l.Subtype != "success" {
			e.Reason, e.Error = harness.Failed, l.Result
			if l.Subtype == "error_during_execution" && l.Result == "Interrupted by user" {
				e.Reason, e.Error = harness.Interrupted, ""
			} else if e.Error == "" {
				e.Error = l.Subtype
			}
		}
		emit(e)
	}
}

func (p *claudeParser) recordUsage(id, model string, u claudeTokens, emit func(harness.Event)) {
	if id == "" || p.usageSeen[id] {
		return
	}
	if id != p.usageID {
		p.flushUsage(emit)
		p.usageID = id
	}
	if model != "" {
		p.usage.Model = model
	}
	p.usage.Input = max(p.usage.Input, u.Input)
	p.usage.Output = max(p.usage.Output, u.Output)
	p.usage.CacheRead = max(p.usage.CacheRead, u.CacheRead)
	p.usage.CacheWrite = max(p.usage.CacheWrite, u.CacheWrite)
}

func (p *claudeParser) flushUsage(emit func(harness.Event)) {
	if p.usageID == "" {
		return
	}
	if p.usageSeen == nil {
		p.usageSeen = map[string]bool{}
	}
	p.usageSeen[p.usageID] = true
	if p.usage.Input+p.usage.Output+p.usage.CacheRead+p.usage.CacheWrite > 0 {
		u := p.usage
		emit(u)
	}
	p.usageID, p.usage = "", harness.StepUsage{}
}

func (p *claudeParser) partial(l claudeLine, emit func(harness.Event)) {
	e := l.Event
	switch e.Type {
	case "message_start":
		p.current = e.Message.ID
	case "content_block_start":
		kind := map[string]harness.TextKind{"text": harness.Assistant, "thinking": harness.Reasoning}[e.ContentBlock.Type]
		if kind == "" {
			return
		}
		id := fmt.Sprintf("%s:%d", p.current, e.Index)
		p.texts[id] = kit.NewText(id, p.run, kind)
		p.texts[id].Append(e.ContentBlock.Text+e.ContentBlock.Thinking, emit)
	case "content_block_delta":
		m := p.texts[fmt.Sprintf("%s:%d", p.current, e.Index)]
		if m == nil {
			return
		}
		m.Append(e.Delta.Text+e.Delta.Thinking, emit)
	}
}

func (p *claudeParser) block(id string, b claudeBlock, emit func(harness.Event)) {
	switch b.Type {
	case "text", "thinking":
		text := p.texts[id]
		if text == nil {
			kind := harness.Assistant
			if b.Type == "thinking" {
				kind = harness.Reasoning
			}
			text = kit.NewText(id, p.run, kind)
		}
		delete(p.texts, id)
		final := b.Text + b.Thinking
		if final != "" || text.String() != "" {
			emit(text.Done(final))
		}
	case "tool_use":
		m := &harness.Message{ID: b.ID, RunID: p.run, Role: "tool", Title: b.Name, Detail: kit.ToolDetail(b.Name, b.Input), Status: "running", CreatedAt: kit.Now(), Tool: kit.ToolCallFromInput(b.Name, b.Input)}
		switch m.Tool.Kind {
		case "todo":
			m.Role = "plan"
		case "plan":
			m.Role, m.Text, m.Status = "plan", m.Tool.Summary, ""
		}
		m.InputChars = len(b.Input)
		p.tools[b.ID] = m
		emit(kit.StartTool(m))
	}
}

func withAnswers(qs []harness.Question, answers map[string]string) []harness.Question {
	out := append([]harness.Question(nil), qs...)
	for i, q := range out {
		if a, ok := answers[q.Question]; ok {
			if q.MultiSelect {
				out[i].Answer = strings.Split(a, ", ")
			} else {
				out[i].Answer = []string{a}
			}
		}
	}
	return out
}

func (c Claude) Info() harness.Info {
	return harness.Info{ID: c.ID(), Driver: "claude", Name: "Claude Code", Order: 0,
		Capabilities: harness.Capabilities{Resume: true, MidTurnRequests: true, UsageReport: true, SystemPrompt: true, Effort: true, OfflineHistory: true, DeferredTools: true, LoginCode: true}}
}

func (c Claude) WithPool(p *procpool.Pool) harness.Driver {
	c.pool = p
	c.cache = &kit.SummaryCache{}
	c.history = &kit.UsageCache{}
	return c
}
func (c Claude) WithAccount(a harness.Account) harness.Driver {
	c.Account, c.Home, c.Label = a.Slug, a.Home, a.Name
	return c
}
func (Claude) ConfigEnv() string              { return "CLAUDE_CONFIG_DIR" }
func (Claude) DefaultHome(home string) string { return filepath.Join(home, ".claude") }
func (c Claude) AccountHome() string          { return c.Home }
func (Claude) ProcessNames() []string         { return []string{"claude"} }
func (Claude) LatestVersion(ctx context.Context) string {
	return update.NPMLatest(ctx, "@anthropic-ai/claude-code")
}
func (c Claude) Commands(p harness.Paths, dir string) (commands, skills []harness.Entry) {
	commands = claudeCommands()
	for _, root := range []string{filepath.Join(kit.AccountDir(p, c), "commands"), filepath.Join(dir, ".claude", "commands")} {
		commands = append(commands, kit.MarkdownEntries(root, "/")...)
	}
	for _, root := range []string{filepath.Join(kit.AccountDir(p, c), "skills"), filepath.Join(dir, ".claude", "skills")} {
		// Claude Code runs a skill as a slash command.
		skills = append(skills, kit.SkillEntries(root, "/")...)
	}
	return
}

func claudeCommands() []harness.Entry {
	return []harness.Entry{
		{Name: "compact", Description: i18n.Text("command.compact"), Insert: "/compact"},
		{Name: "context", Description: i18n.Text("command.context"), Insert: "/context"},
		{Name: "cost", Description: i18n.Text("command.cost"), Insert: "/cost"},
		{Name: "init", Description: i18n.Text("command.init"), Insert: "/init"},
		{Name: "review", Description: i18n.Text("command.review"), Insert: "/review"},
		{Name: "security-review", Description: i18n.Text("command.security_review"), Insert: "/security-review"},
		{Name: "pr-comments", Description: i18n.Text("command.pr_comments"), Insert: "/pr-comments"},
	}
}

func (c Claude) SkillDir(p harness.Paths) harness.SkillLocation {
	return harness.SkillLocation{Dir: filepath.Join(kit.AccountDir(p, c), "skills")}
}
