package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
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

// Start runs a turn with stream-json on both ends. The process asks Dory over
// stdio before using a tool, so AskUserQuestion pauses the turn until the
// user answers it. Stdin closes when the turn ends, which ends the process.
func (c Claude) Start(ctx context.Context, t Turn) (Session, error) {
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
			"type": "http", "url": t.MCPURL, "headers": map[string]string{"Authorization": "Bearer " + t.MCPToken},
		}}})
		args = append(args, "--mcp-config", string(cfg))
	}
	cmd := exec.CommandContext(ctx, c.bin(), args...)
	cmd.Dir = t.Dir
	// ask_user waits for the user, so an MCP call may take long (ms).
	cmd.Env = append(os.Environ(), "MCP_TOOL_TIMEOUT=86400000")
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	s := newClaudeSession(t.RunID, in, t.FullAccess)
	if s.events, err = pipe(ctx, cmd, s.handle); err != nil {
		return nil, err
	}
	if err = s.write(map[string]any{"type": "user", "parent_tool_use_id": nil,
		"message": map[string]any{"role": "user", "content": t.Text}}); err != nil {
		return nil, err
	}
	return s, nil
}

// claudeSession speaks Claude Code's control protocol on stdin while its
// parser follows stdout.
type claudeSession struct {
	parser     *claudeParser
	events     <-chan Event
	fullAccess bool

	mu      sync.Mutex
	in      io.WriteCloser
	pending map[string]claudeAsk
}

// claudeAsk is an AskUserQuestion call waiting for the user.
type claudeAsk struct {
	input     map[string]any
	questions []Question
}

func newClaudeSession(run string, in io.WriteCloser, fullAccess bool) *claudeSession {
	return &claudeSession{parser: &claudeParser{run: run, texts: map[string]*Message{}, tools: map[string]*Message{}},
		in: in, fullAccess: fullAccess, pending: map[string]claudeAsk{}}
}

func (s *claudeSession) Events() <-chan Event { return s.events }

// Answer lets the pending AskUserQuestion run with the answers, keyed by
// question text; several choices go joined by ", " as Claude expects.
func (s *claudeSession) Answer(id string, answers [][]string) error {
	s.mu.Lock()
	ask, ok := s.pending[id]
	delete(s.pending, id)
	s.mu.Unlock()
	if !ok {
		return ErrNoRequest
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

func (s *claudeSession) handle(line []byte, emit func(Event)) {
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
		if l.Type == "result" {
			s.mu.Lock()
			s.in.Close()
			s.mu.Unlock()
		}
	}
}

// control answers a permission request of the CLI. AskUserQuestion becomes
// a pending request; other tools follow the access mode of the thread.
func (s *claudeSession) control(l claudeLine, emit func(Event)) {
	r := l.Request
	if r.Subtype != "can_use_tool" {
		s.write(map[string]any{"type": "control_response", "response": map[string]any{
			"subtype": "error", "request_id": l.RequestID, "error": "unsupported request: " + r.Subtype}})
		return
	}
	var input map[string]any
	_ = json.Unmarshal(r.Input, &input)
	if r.ToolName == "AskUserQuestion" {
		qs := parseQuestions(r.Input)
		s.mu.Lock()
		s.pending[l.RequestID] = claudeAsk{input: input, questions: qs}
		s.mu.Unlock()
		emit(Event{Request: &Request{ID: l.RequestID, Kind: "question", MessageID: r.ToolUseID, Questions: qs, CreatedAt: now()}})
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
				} else if m.Tool != nil && m.Tool.Kind == "question" && len(l.ToolUseResult.Answers) > 0 {
					m.Tool.Questions = withAnswers(m.Tool.Questions, l.ToolUseResult.Answers)
					m.Text = ""
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

func copyOf(m *Message) *Message {
	c := *m
	if m.Tool != nil {
		t := *m.Tool
		t.Questions = append([]Question(nil), t.Questions...)
		c.Tool = &t
	}
	return &c
}

// withAnswers sets the answers Claude reports, keyed by question text, on a
// copy of the questions. Several choices come joined by ", ".
func withAnswers(qs []Question, answers map[string]string) []Question {
	out := append([]Question(nil), qs...)
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
