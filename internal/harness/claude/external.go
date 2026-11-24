package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
)

func (c Claude) List(_ context.Context, p harness.Paths) []harness.External {
	return c.cache.List(c.ID(), filepath.Join(kit.AccountDir(p, c), "projects", "*", "*.jsonl"), claudeSummary)
}

func claudeSummary(line []byte, e *harness.External) {
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
		e.Title = kit.Short(text, 80)
	}
}

func (c Claude) Read(_ context.Context, e harness.External) ([]harness.Message, error) {
	return kit.ReadExternal(e, claudeRecord)
}
func claudeRecord(line []byte, emit func(harness.Message), out []harness.Message, index map[string]int) {
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
		if r.Type == "user" && (!strings.HasPrefix(strings.TrimSpace(text), "<") || kit.DoryMessage(strings.TrimSpace(text))) {
			emit(harness.Message{ID: r.UUID, Role: "user", Text: text, CreatedAt: r.Timestamp})
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
				if strings.HasPrefix(strings.TrimSpace(b.Text), "<") && !kit.DoryMessage(strings.TrimSpace(b.Text)) {
					continue
				}
				role = "user"
			}
			emit(harness.Message{ID: id, Role: role, Text: b.Text, CreatedAt: r.Timestamp})
		case "thinking":
			if b.Thinking != "" {
				emit(harness.Message{ID: id, Role: "reasoning", Title: "Pensando", Text: b.Thinking, CreatedAt: r.Timestamp})
			}
		case "tool_use":
			emit(harness.Message{ID: b.ID, Role: "tool", Title: b.Name, Detail: kit.ToolDetail(b.Name, b.Input), Status: "completed", CreatedAt: r.Timestamp, Tool: kit.ToolCallFromInput(b.Name, b.Input)})
		case "tool_result":
			if i, ok := index[b.ToolUseID]; ok {
				m := out[i]
				m.Text = kit.Clip(kit.ResultText(b.Content), 4000)
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

func (c Claude) SessionFile(p harness.Paths, id string) string {
	return kit.FirstMatch(filepath.Join(kit.AccountDir(p, c), "projects", "*", id+".jsonl"))
}
