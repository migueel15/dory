package runtime

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// External is a conversation a harness saved on this machine, started from
// the harness itself rather than from Dory.
type External struct {
	Provider  string `json:"provider"`
	ID        string `json:"id"`
	Title     string `json:"title"`
	Dir       string `json:"dir"`
	Branch    string `json:"branch,omitempty"`
	UpdatedAt string `json:"updated_at"`
	path      string
}

// externalLimit bounds how many recent sessions of each harness are listed.
const externalLimit = 150

type summaryCache struct {
	mu   sync.Mutex
	seen map[string]cachedSummary
}

type cachedSummary struct {
	mod time.Time
	ext External
}

var summaries = summaryCache{seen: map[string]cachedSummary{}}

// ExternalSessions lists the recent conversations of Claude Code and Codex on
// this machine, newest first.
func ExternalSessions(home string) []External {
	var out []External
	for provider, pattern := range map[string]string{
		"claude": filepath.Join(home, ".claude", "projects", "*", "*.jsonl"),
		"codex":  filepath.Join(home, ".codex", "sessions", "*", "*", "*", "rollout-*.jsonl"),
	} {
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
			e, ok := summarize(provider, f.path, f.mod)
			if ok {
				out = append(out, e)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out
}

// summarize reads the title, folder and branch of a session file, cached by
// modification time because files can be large.
func summarize(provider, path string, mod time.Time) (External, bool) {
	summaries.mu.Lock()
	c, hit := summaries.seen[path]
	summaries.mu.Unlock()
	if hit && c.mod.Equal(mod) {
		return c.ext, c.ext.ID != ""
	}
	e := External{Provider: provider, path: path, UpdatedAt: mod.UTC().Format(time.RFC3339)}
	f, err := os.Open(path)
	if err != nil {
		return e, false
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64<<10), 64<<20)
	for scan.Scan() {
		line := scan.Bytes()
		switch provider {
		case "claude":
			claudeSummary(line, &e)
		case "codex":
			codexSummary(line, &e)
		}
	}
	if e.Title == "" {
		e.ID = ""
	}
	summaries.mu.Lock()
	summaries.seen[path] = cachedSummary{mod, e}
	summaries.mu.Unlock()
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

// ExternalSession finds one session by id.
func ExternalSession(home, provider, id string) (External, error) {
	for _, e := range ExternalSessions(home) {
		if e.Provider == provider && e.ID == id {
			return e, nil
		}
	}
	return External{}, fmt.Errorf("%w: %s session %s", ErrNotFound, provider, id)
}

// ExternalMessages reads the conversation of a session as Dory messages.
func ExternalMessages(e External) ([]Message, error) {
	f, err := os.Open(e.path)
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
		switch e.Provider {
		case "claude":
			claudeRecord(scan.Bytes(), emit, out, index)
		case "codex":
			codexRecord(scan.Bytes(), emit, out, index)
		default:
			return nil, errors.New("unknown provider")
		}
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
			emit(Message{ID: id, Role: "assistant", Text: text, CreatedAt: r.Timestamp})
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
			switch o := p.Output.(type) {
			case string:
				m.Text = clip(o, 4000)
				if m.Tool != nil && m.Tool.Kind == "question" {
					t := *m.Tool
					if qs, ok := codexAnswers(t.Questions, o); ok {
						t.Questions, m.Tool, m.Text = qs, &t, ""
					}
				}
			default:
				raw, _ := json.Marshal(o)
				m.Text = clip(string(raw), 4000)
			}
			emit(m)
		}
	}
}

// sessionFile finds the file where a harness keeps a session.
func sessionFile(home, provider, id string) string {
	var pattern string
	switch provider {
	case "claude":
		pattern = filepath.Join(home, ".claude", "projects", "*", id+".jsonl")
	case "codex":
		pattern = filepath.Join(home, ".codex", "sessions", "*", "*", "*", "rollout-*"+id+".jsonl")
	default:
		return ""
	}
	files, _ := filepath.Glob(pattern)
	if len(files) == 0 {
		return ""
	}
	return files[0]
}
