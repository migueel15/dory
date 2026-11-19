package runtime

import (
	"context"
	"encoding/json"
	"strings"
)

// Codex runs turns over "codex app-server" (codex_app.go); a later turn
// resumes the same thread. codexParser maps the items of "codex exec --json",
// which app-server items are converted to.
type Codex struct{ Bin string }

func (Codex) ID() string { return "codex" }

func (c Codex) bin() string {
	if c.Bin != "" {
		return c.Bin
	}
	return resolveBin("codex")
}

func (c Codex) Detect(ctx context.Context) Status {
	version, ok := detect(ctx, c.bin())
	efforts := []string{"low", "medium", "high", "xhigh"}
	status := Status{ID: "codex", Name: "Codex", Installed: ok, Version: version,
		Models:       []Model{{Slug: "gpt-5.5", Name: "GPT-5.5", Efforts: efforts}},
		Capabilities: []string{"interrupt", "resume", "effort", "usage"},
	}
	if ok {
		if models := codexModels(ctx, c.bin()); len(models) > 0 {
			status.Models = models
		}
	}
	return status
}

type codexParser struct {
	run     string
	started bool
}

type codexItem struct {
	ID               string          `json:"id"`
	Type             string          `json:"type"`
	Text             string          `json:"text"`
	Message          string          `json:"message"`
	Command          string          `json:"command"`
	AggregatedOutput string          `json:"aggregated_output"`
	ExitCode         *int            `json:"exit_code"`
	Status           string          `json:"status"`
	Server           string          `json:"server"`
	Tool             string          `json:"tool"`
	Query            string          `json:"query"`
	Arguments        json.RawMessage `json:"arguments"`
	Changes          []struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
	} `json:"changes"`
	Items []struct {
		Text      string `json:"text"`
		Completed bool   `json:"completed"`
	} `json:"items"`
}

func (p *codexParser) parse(line []byte, emit func(Event)) {
	var l struct {
		Type     string    `json:"type"`
		ThreadID string    `json:"thread_id"`
		Item     codexItem `json:"item"`
		Error    struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(line, &l) != nil {
		return
	}
	switch l.Type {
	case "thread.started":
		emit(Event{Session: l.ThreadID})
	case "turn.started":
		p.started = true
	case "item.started", "item.updated", "item.completed":
		// Errors before the turn are configuration warnings, not part of it.
		if !p.started && l.Item.Type == "error" {
			return
		}
		if m := p.message(l.Item, l.Type == "item.completed"); m != nil {
			emit(Event{Message: m})
		}
	case "turn.completed":
		emit(Event{Done: true})
	case "turn.failed":
		emit(Event{Done: true, Err: l.Error.Message})
	case "error":
		emit(Event{Done: true, Err: l.Message})
	}
}

func (p *codexParser) message(it codexItem, done bool) *Message {
	m := &Message{ID: p.run + ":" + it.ID, RunID: p.run, CreatedAt: now(), Streaming: !done}
	status := "running"
	if done {
		status = "completed"
	}
	switch it.Type {
	case "agent_message":
		m.Role, m.Text = "assistant", it.Text
	case "reasoning":
		m.Role, m.Title, m.Text = "reasoning", "Pensando", it.Text
	case "command_execution":
		m.Role, m.Title, m.Detail, m.Text = "tool", "Bash", short(it.Command, 300), clip(it.AggregatedOutput, 4000)
		m.Tool = &ToolCall{Kind: "command", Name: "command_execution", Command: it.Command, ExitCode: it.ExitCode}
		if it.ExitCode != nil && *it.ExitCode != 0 || it.Status == "failed" {
			status = "failed"
		}
	case "file_change":
		var paths []string
		for _, c := range it.Changes {
			paths = append(paths, c.Kind+" "+c.Path)
		}
		m.Role, m.Title, m.Detail = "tool", "Editar ficheros", strings.Join(paths, "\n")
		m.Tool = &ToolCall{Kind: "edit", Name: "file_change"}
		for _, c := range it.Changes {
			m.Tool.Files = append(m.Tool.Files, FileEdit{Path: c.Path, Change: c.Kind})
		}
	case "mcp_tool_call":
		m.Role, m.Title = "tool", it.Server+"."+it.Tool
		m.Tool = &ToolCall{Kind: "mcp", Name: it.Tool, Server: it.Server, Tool: it.Tool}
		if len(it.Arguments) > 0 && string(it.Arguments) != "null" {
			m.Tool.Input = clip(string(it.Arguments), 4000)
		}
	case "web_search":
		m.Role, m.Title, m.Detail = "tool", "Buscar en la web", it.Query
		m.Tool = &ToolCall{Kind: "web", Name: "web_search", Query: it.Query}
	case "todo_list":
		var lines []string
		for _, t := range it.Items {
			mark := "[ ]"
			if t.Completed {
				mark = "[x]"
			}
			lines = append(lines, mark+" "+t.Text)
		}
		m.Role, m.Title, m.Detail = "plan", "Plan", strings.Join(lines, "\n")
		m.Tool = &ToolCall{Kind: "todo", Name: "todo_list"}
		for _, t := range it.Items {
			st := "pending"
			if t.Completed {
				st = "completed"
			}
			m.Tool.Todos = append(m.Tool.Todos, Todo{Text: t.Text, Status: st})
		}
	case "error":
		m.Role, m.Text, status = "system", it.Message, "failed"
	default:
		return nil
	}
	if m.Role == "tool" || m.Role == "plan" {
		m.Status = status
	}
	if m.Role != "assistant" && m.Role != "reasoning" {
		m.Streaming = false
	}
	return m
}
