package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/harness"
)

type External = harness.External

// externalLimit bounds how many recent sessions of each harness are listed.
const externalLimit = 150

type summaryCache struct {
	mu   sync.Mutex
	seen map[summaryKey]cachedSummary
}

type summaryKey struct{ provider, path string }

type cachedSummary struct {
	mod time.Time
	ext External
}

type sessionParser func([]byte, *External)
type recordParser func([]byte, func(Message), []Message, map[string]int)

func (c Claude) List(_ context.Context, p Paths) []External {
	return c.cache.list(c.ID(), filepath.Join(accountDir(p, c), "projects", "*", "*.jsonl"), claudeSummary)
}

func (c Codex) List(_ context.Context, p Paths) []External {
	return c.cache.list(c.ID(), filepath.Join(accountDir(p, c), "sessions", "*", "*", "*", "rollout-*.jsonl"), codexSummary)
}

func (c *summaryCache) list(provider, pattern string, parse sessionParser) []External {
	if c == nil {
		c = &summaryCache{seen: map[summaryKey]cachedSummary{}}
	}
	var out []External
	files, _ := filepath.Glob(pattern)
	type file struct {
		path string
		mod  time.Time
	}
	var list []file
	for _, f := range files {
		if st, err := os.Stat(f); err == nil {
			list = append(list, file{f, st.ModTime()})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].mod.After(list[j].mod) })
	if len(list) > externalLimit {
		list = list[:externalLimit]
	}
	for _, f := range list {
		if e, ok := c.summarize(provider, f.path, f.mod, parse); ok {
			out = append(out, e)
		}
	}
	return out
}

// summarize reads the title, folder and branch of a session file, cached by
// modification time because files can be large.
func (cache *summaryCache) summarize(provider, path string, mod time.Time, parse sessionParser) (External, bool) {
	cache.mu.Lock()
	c, hit := cache.seen[summaryKey{provider, path}]
	cache.mu.Unlock()
	if hit && c.mod.Equal(mod) {
		return c.ext, c.ext.ID != ""
	}
	e := External{Provider: provider, Path: path, UpdatedAt: mod.UTC().Format(time.RFC3339)}
	f, err := os.Open(path)
	if err != nil {
		return e, false
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64<<10), 64<<20)
	for scan.Scan() {
		parse(scan.Bytes(), &e)
	}
	if e.Title == "" {
		e.ID = ""
	}
	cache.mu.Lock()
	cache.seen[summaryKey{provider, path}] = cachedSummary{mod, e}
	cache.mu.Unlock()
	return e, e.ID != ""
}

func claudeSummary(line []byte, e *External) {
	if bytes.Contains(line, []byte(`"type":"custom-title"`)) || bytes.Contains(line, []byte(`"type":"ai-title"`)) {
		var t struct {
			CustomTitle string `json:"customTitle"`
			AITitle     string `json:"aiTitle"`
		}
		if json.Unmarshal(line, &t) == nil && t.CustomTitle+t.AITitle != "" {
			e.Title = t.CustomTitle + t.AITitle
		}
		return
	}
	if e.ID != "" && e.Title != "" || !bytes.Contains(line, []byte(`"type":"user"`)) {
		return
	}
	var r struct {
		Type        string `json:"type"`
		SessionID   string `json:"sessionId"`
		Cwd         string `json:"cwd"`
		GitBranch   string `json:"gitBranch"`
		IsSidechain bool   `json:"isSidechain"`
		Message     struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &r) != nil || r.Type != "user" || r.IsSidechain {
		return
	}
	e.ID, e.Dir, e.Branch = r.SessionID, r.Cwd, r.GitBranch
	var text string
	if json.Unmarshal(r.Message.Content, &text) == nil && e.Title == "" && !strings.HasPrefix(strings.TrimSpace(text), "<") {
		e.Title = short(text, 80)
	}
}

func codexSummary(line []byte, e *External) {
	var r struct {
		Type    string `json:"type"`
		Payload struct {
			Type      string `json:"type"`
			Role      string `json:"role"`
			SessionID string `json:"session_id"`
			ID        string `json:"id"`
			Cwd       string `json:"cwd"`
			Git       *struct {
				Branch string `json:"branch"`
			} `json:"git"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"payload"`
	}
	if json.Unmarshal(line, &r) != nil {
		return
	}
	p := r.Payload
	switch {
	case r.Type == "session_meta":
		e.ID, e.Dir = p.SessionID, p.Cwd
		if e.ID == "" {
			e.ID = p.ID
		}
		if p.Git != nil {
			e.Branch = p.Git.Branch
		}
	case e.Title == "" && p.Type == "message" && p.Role == "user" && len(p.Content) > 0 && !codexContext(p.Content[0].Text):
		e.Title = short(p.Content[0].Text, 80)
	}
}

// codexContext tells the instructions Codex injects as user messages from
// what the user wrote.
func codexContext(text string) bool {
	t := strings.TrimSpace(text)
	return !doryMessage(t) && (strings.HasPrefix(t, "<") || strings.HasPrefix(t, "# AGENTS.md"))
}

// Dory's generated context is part of the user's message, not a harness
// instruction. Keep it intact so the client can display the user's text.
func doryMessage(text string) bool {
	return strings.HasPrefix(text, "<dory-context ") || strings.HasPrefix(text, "<dory-plan ")
}

func (c Claude) Read(_ context.Context, e External) ([]Message, error) {
	return readExternal(e, claudeRecord)
}
func (c Codex) Read(_ context.Context, e External) ([]Message, error) {
	return readExternal(e, codexRecord)
}

func readExternal(e External, parse recordParser) ([]Message, error) {
	f, err := os.Open(e.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64<<10), 64<<20)
	var out []Message
	index := map[string]int{}
	emit := func(m Message) {
		if i, ok := index[m.ID]; ok {
			out[i] = m
			return
		}
		index[m.ID] = len(out)
		out = append(out, m)
	}
	for scan.Scan() {
		parse(scan.Bytes(), emit, out, index)
	}
	return out, scan.Err()
}

func claudeRecord(line []byte, emit func(Message), out []Message, index map[string]int) {
	var r struct {
		Type        string `json:"type"`
		UUID        string `json:"uuid"`
		Timestamp   string `json:"timestamp"`
		IsSidechain bool   `json:"isSidechain"`
		IsMeta      bool   `json:"isMeta"`
		// ToolUseResult carries the answers of an AskUserQuestion result.
		ToolUseResult json.RawMessage `json:"toolUseResult"`
		Message       struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &r) != nil || r.IsSidechain || r.IsMeta || r.Type != "user" && r.Type != "assistant" {
		return
	}
	var text string
	if json.Unmarshal(r.Message.Content, &text) == nil {
		if r.Type == "user" && (!strings.HasPrefix(strings.TrimSpace(text), "<") || doryMessage(strings.TrimSpace(text))) {
			emit(Message{ID: r.UUID, Role: "user", Text: text, CreatedAt: r.Timestamp})
		}
		return
	}
	var blocks []claudeBlock
	if json.Unmarshal(r.Message.Content, &blocks) != nil {
		return
	}
	for i, b := range blocks {
		id := fmt.Sprintf("%s:%d", r.UUID, i)
		switch b.Type {
		case "text":
			role := "assistant"
			if r.Type == "user" {
				if strings.HasPrefix(strings.TrimSpace(b.Text), "<") && !doryMessage(strings.TrimSpace(b.Text)) {
					continue
				}
				role = "user"
			}
			emit(Message{ID: id, Role: role, Text: b.Text, CreatedAt: r.Timestamp})
		case "thinking":
			if b.Thinking != "" {
				emit(Message{ID: id, Role: "reasoning", Title: "Pensando", Text: b.Thinking, CreatedAt: r.Timestamp})
			}
		case "tool_use":
			emit(Message{ID: b.ID, Role: "tool", Title: b.Name, Detail: toolDetail(b.Name, b.Input), Status: "completed", CreatedAt: r.Timestamp, Tool: toolCall(b.Name, b.Input)})
		case "tool_result":
			if i, ok := index[b.ToolUseID]; ok {
				m := out[i]
				m.Text = clip(resultText(b.Content), 4000)
				if b.IsError {
					m.Status = "failed"
				}
				var result struct {
					Answers map[string]string `json:"answers"`
				}
				if m.Tool != nil && m.Tool.Kind == "question" && json.Unmarshal(r.ToolUseResult, &result) == nil && len(result.Answers) > 0 {
					t := *m.Tool
					t.Questions = withAnswers(t.Questions, result.Answers)
					m.Tool, m.Text = &t, ""
				}
				emit(m)
			}
		}
	}
}

func codexRecord(line []byte, emit func(Message), out []Message, index map[string]int) {
	var r struct {
		Timestamp string `json:"timestamp"`
		Type      string `json:"type"`
		Payload   struct {
			Type      string `json:"type"`
			ID        string `json:"id"`
			Role      string `json:"role"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			Input     string `json:"input"`
			CallID    string `json:"call_id"`
			Output    any    `json:"output"`
			Content   []struct {
				Text string `json:"text"`
			} `json:"content"`
			Summary []struct {
				Text string `json:"text"`
			} `json:"summary"`
		} `json:"payload"`
	}
	if json.Unmarshal(line, &r) != nil || r.Type != "response_item" {
		return
	}
	p := r.Payload
	id := p.ID
	if id == "" {
		id = fmt.Sprintf("%s:%s", r.Timestamp, p.Type)
	}
	switch p.Type {
	case "message":
		var parts []string
		for _, c := range p.Content {
			parts = append(parts, c.Text)
		}
		text := strings.Join(parts, "\n")
		switch {
		case p.Role == "assistant":
			emit(Message{ID: id, Role: "assistant", Text: stripCodexMemoryCitations(text, false), CreatedAt: r.Timestamp})
		case p.Role == "user" && !codexContext(text):
			emit(Message{ID: id, Role: "user", Text: text, CreatedAt: r.Timestamp})
		}
	case "reasoning":
		var parts []string
		for _, s := range p.Summary {
			parts = append(parts, s.Text)
		}
		if len(parts) > 0 {
			emit(Message{ID: id, Role: "reasoning", Title: "Pensando", Text: strings.Join(parts, "\n"), CreatedAt: r.Timestamp})
		}
	case "function_call", "custom_tool_call":
		args := p.Arguments + p.Input
		detail := toolDetail(p.Name, json.RawMessage(args))
		if detail == "" {
			detail = short(args, 300)
		}
		emit(Message{ID: p.CallID, Role: "tool", Title: p.Name, Detail: detail, Status: "completed", CreatedAt: r.Timestamp, Tool: codexTool(p.Name, args)})
	case "function_call_output", "custom_tool_call_output":
		if i, ok := index[p.CallID]; ok {
			m := out[i]
			o := codexOutput(p.Output)
			m.Text = clip(o, 4000)
			if _, ok := p.Output.(string); ok && m.Tool != nil && m.Tool.Kind == "question" {
				t := *m.Tool
				if qs, ok := codexAnswers(t.Questions, o); ok {
					t.Questions, m.Tool, m.Text = qs, &t, ""
				}
			}
			emit(m)
		}
	}
}

// sessionFile finds the file where a harness keeps a session.
func (c Claude) SessionFile(p Paths, id string) string {
	return firstMatch(filepath.Join(accountDir(p, c), "projects", "*", id+".jsonl"))
}
func (c Codex) SessionFile(p Paths, id string) string {
	return firstMatch(filepath.Join(accountDir(p, c), "sessions", "*", "*", "*", "rollout-*"+id+".jsonl"))
}
func firstMatch(pattern string) string {
	files, _ := filepath.Glob(pattern)
	if len(files) == 0 {
		return ""
	}
	return files[0]
}

func (m *Manager) ListExternal(ctx context.Context, p Paths) []External {
	var out []External
	for _, d := range m.registry.All() {
		if d.External != nil && (m.Enabled(d.Info().ID) || d.Info().Capabilities.OfflineHistory) {
			out = append(out, d.External.List(ctx, p)...)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out
}

func (m *Manager) FindExternal(ctx context.Context, p Paths, provider, id string) (External, error) {
	if d, ok := m.registry.Get(provider); ok && d.External != nil && (m.Enabled(provider) || d.Info().Capabilities.OfflineHistory) {
		for _, e := range d.External.List(ctx, p) {
			if e.ID == id {
				return e, nil
			}
		}
	}
	return External{}, ErrNotFound.Msgf("%s session %s", provider, id)
}

func (m *Manager) ReadExternal(ctx context.Context, e External) ([]Message, error) {
	if d, ok := m.registry.Get(e.Provider); ok && d.External != nil && (m.Enabled(e.Provider) || d.Info().Capabilities.OfflineHistory) {
		return d.External.Read(ctx, e)
	}
	return nil, ErrProvider
}
