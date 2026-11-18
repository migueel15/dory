package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Claude runs Claude Code headless with its stream-json output.
type Claude struct{ Bin string }

func (Claude) ID() string { return "claude" }

func (c Claude) bin() string {
	if c.Bin != "" {
		return c.Bin
	}
	return "claude"
}

func (c Claude) Detect(ctx context.Context) Status {
	version, ok := detect(ctx, c.bin())
	status := Status{ID: "claude", Name: "Claude Code", Installed: ok, Version: version,
		Models: []Model{
			{Slug: "opus", Name: "Opus", Efforts: []string{"low", "medium", "high", "xhigh", "max"}},
			{Slug: "sonnet", Name: "Sonnet", Efforts: []string{"low", "medium", "high", "xhigh", "max"}},
			{Slug: "haiku", Name: "Haiku"},
		},
		Capabilities: []string{"interrupt", "resume", "effort", "usage"},
	}
	if ok {
		if models := claudeModels(ctx, c.bin()); len(models) > 0 {
			status.Models = models
		}
	}
	return status
}

func (c Claude) Run(ctx context.Context, t Turn) (<-chan Event, error) {
	args := []string{"-p", "--output-format", "stream-json", "--verbose", "--include-partial-messages"}
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
			"type": "http", "url": t.MCPURL, "headers": map[string]string{"Authorization": "Bearer " + t.MCPToken},
		}}})
		args = append(args, "--mcp-config", string(cfg))
		// In -p mode Claude's own AskUserQuestion cannot reach the user, so it
		// asks through Dory's ask_user, which the chat shows with its options.
		args = append(args, "--disallowedTools", "AskUserQuestion",
			"--append-system-prompt", "To ask the user anything, call the dory ask_user tool (same questions/options shape as AskUserQuestion), then end your turn: the answer arrives as the next user message.")
	}
	cmd := exec.CommandContext(ctx, c.bin(), args...)
	cmd.Dir = t.Dir
	p := &claudeParser{run: t.RunID, texts: map[string]*Message{}, tools: map[string]*Message{}}
	return stream(ctx, cmd, t.Text, p.parse)
}

// claudeParser follows one turn. Streamed blocks are keyed by message id and
// block index, which the final assistant event repeats.
type claudeParser struct {
	run     string
	current string
	texts   map[string]*Message
	tools   map[string]*Message
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

type claudeLine struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	SessionID string `json:"session_id"`
	Model     string `json:"model"`
	Effort    string `json:"effort"`
	IsError   bool   `json:"is_error"`
	Result    string `json:"result"`
	Event     struct {
		Type    string `json:"type"`
		Index   int    `json:"index"`
		Message struct {
			ID string `json:"id"`
		} `json:"message"`
		ContentBlock claudeBlock `json:"content_block"`
		Delta        struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
		} `json:"delta"`
	} `json:"event"`
	Message struct {
		ID      string        `json:"id"`
		Model   string        `json:"model"`
		Content []claudeBlock `json:"content"`
	} `json:"message"`
	ParentToolUseID *string `json:"parent_tool_use_id"`
	RateLimitInfo   struct {
		Unified map[string]struct {
			Utilization float64 `json:"utilization"`
			ResetsAt    int64   `json:"resetsAt"`
		} `json:"unifiedWindows"`
	} `json:"rate_limit_info"`
}

func (p *claudeParser) parse(line []byte, emit func(Event)) {
	var l claudeLine
	if json.Unmarshal(line, &l) != nil {
		return
	}
	switch l.Type {
	case "system":
		if l.Subtype == "init" {
			if l.SessionID != "" {
				emit(Event{Session: l.SessionID})
			}
			if l.Model != "" || l.Effort != "" {
				emit(Event{Settings: &ReportedSettings{Model: l.Model, Effort: l.Effort}})
			}
		}
	case "stream_event":
		if l.ParentToolUseID == nil {
			p.partial(l, emit)
		}
	case "assistant":
		if l.ParentToolUseID == nil && l.Message.Model != "" && l.Message.Model != "<synthetic>" {
			emit(Event{Settings: &ReportedSettings{Model: l.Message.Model}})
		}
		// A subagent's own work stays inside its card: it only counts its calls.
		if l.ParentToolUseID != nil {
			if parent := p.tools[*l.ParentToolUseID]; parent != nil && parent.Tool != nil {
				for _, b := range l.Message.Content {
					if b.Type == "tool_use" {
						parent.Tool.Steps++
					}
				}
				emit(Event{Message: copyOf(parent)})
			}
			return
		}
		for i, b := range l.Message.Content {
			p.block(fmt.Sprintf("%s:%d", l.Message.ID, i), b, emit)
		}
	case "user":
		for _, b := range l.Message.Content {
			if b.Type != "tool_result" {
				continue
			}
			if m := p.tools[b.ToolUseID]; m != nil {
				m.Status = "completed"
				if b.IsError {
					m.Status = "failed"
				}
				if m.Role == "plan" {
					m.Status = "" // the plan keeps its text; the result is only an ack
				} else {
					m.Text = clip(resultText(b.Content), 4000)
				}
				emit(Event{Message: copyOf(m)})
			}
		}
	case "rate_limit_event":
		var windows []Window
		for kind, w := range l.RateLimitInfo.Unified {
			windows = append(windows, Window{Kind: kind, Utilization: w.Utilization, ResetsAt: time.Unix(w.ResetsAt, 0).UTC()})
		}
		sort.Slice(windows, func(i, j int) bool { return windows[i].Kind < windows[j].Kind })
		if len(windows) > 0 {
			emit(Event{Usage: windows})
		}
	case "result":
		for _, m := range p.texts {
			if m.Streaming {
				m.Streaming = false
				emit(Event{Message: copyOf(m)})
			}
		}
		e := Event{Done: true}
		if l.IsError || l.Subtype != "success" {
			e.Err = l.Result
			if e.Err == "" {
				e.Err = l.Subtype
			}
		}
		emit(e)
	}
}

func (p *claudeParser) partial(l claudeLine, emit func(Event)) {
	e := l.Event
	switch e.Type {
	case "message_start":
		p.current = e.Message.ID
	case "content_block_start":
		role := map[string]string{"text": "assistant", "thinking": "reasoning"}[e.ContentBlock.Type]
		if role == "" {
			return
		}
		id := fmt.Sprintf("%s:%d", p.current, e.Index)
		m := &Message{ID: id, RunID: p.run, Role: role, Streaming: true, CreatedAt: now()}
		if role == "reasoning" {
			m.Title = "Pensando"
		}
		p.texts[id] = m
	case "content_block_delta":
		m := p.texts[fmt.Sprintf("%s:%d", p.current, e.Index)]
		if m == nil {
			return
		}
		m.Text += e.Delta.Text + e.Delta.Thinking
		emit(Event{Message: copyOf(m)})
	}
}

func (p *claudeParser) block(id string, b claudeBlock, emit func(Event)) {
	switch b.Type {
	case "text", "thinking":
		m := p.texts[id]
		if m == nil {
			m = &Message{ID: id, RunID: p.run, Role: "assistant", CreatedAt: now()}
			if b.Type == "thinking" {
				m.Role, m.Title = "reasoning", "Pensando"
			}
			p.texts[id] = m
		}
		m.Text = b.Text + b.Thinking
		m.Streaming = false
		if m.Text != "" {
			emit(Event{Message: copyOf(m)})
		}
	case "tool_use":
		m := &Message{ID: b.ID, RunID: p.run, Role: "tool", Title: b.Name, Detail: toolDetail(b.Name, b.Input), Status: "running", CreatedAt: now(), Tool: toolCall(b.Name, b.Input)}
		switch m.Tool.Kind {
		case "todo":
			m.Role = "plan"
		case "plan":
			m.Role, m.Text, m.Status = "plan", m.Tool.Summary, ""
		}
		p.tools[b.ID] = m
		emit(Event{Message: copyOf(m)})
	}
}

// toolDetail picks the most telling argument of a tool call.
func toolDetail(name string, input json.RawMessage) string {
	var args map[string]any
	if json.Unmarshal(input, &args) != nil {
		return ""
	}
	for _, key := range []string{"command", "file_path", "path", "pattern", "url", "query", "description", "prompt"} {
		if v, ok := args[key].(string); ok && v != "" {
			return short(v, 300)
		}
	}
	if name == "TodoWrite" {
		if todos, ok := args["todos"].([]any); ok {
			var lines []string
			for _, t := range todos {
				if m, ok := t.(map[string]any); ok {
					mark := "[ ]"
					switch m["status"] {
					case "completed":
						mark = "[x]"
					case "in_progress":
						mark = "[~]"
					}
					lines = append(lines, fmt.Sprintf("%s %v", mark, m["content"]))
				}
			}
			return strings.Join(lines, "\n")
		}
	}
	return ""
}

// resultText flattens a tool result, which is a string or a list of blocks.
func resultText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	json.Unmarshal(raw, &blocks)
	var parts []string
	for _, b := range blocks {
		if b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func copyOf(m *Message) *Message { c := *m; return &c }
