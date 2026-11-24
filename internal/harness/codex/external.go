package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
)

func (c Codex) List(_ context.Context, p harness.Paths) []harness.External {
	return c.cache.List(c.ID(), filepath.Join(kit.AccountDir(p, c), "sessions", "*", "*", "*", "rollout-*.jsonl"), codexSummary)
}

func codexSummary(line []byte, e *harness.External) {
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
		e.Title = kit.Short(p.Content[0].Text, 80)
	}
}

// codexContext tells the instructions Codex injects as user messages from
// what the user wrote.
func codexContext(text string) bool {
	t := strings.TrimSpace(text)
	return !kit.DoryMessage(t) && (strings.HasPrefix(t, "<") || strings.HasPrefix(t, "# AGENTS.md"))
}

// Dory's generated context is part of the user's message, not a harness
// instruction. Keep it intact so the client can display the user's text.
func (c Codex) Read(_ context.Context, e harness.External) ([]harness.Message, error) {
	return kit.ReadExternal(e, codexRecord)
}

func codexRecord(line []byte, emit func(harness.Message), out []harness.Message, index map[string]int) {
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
			emit(harness.Message{ID: id, Role: "assistant", Text: stripCodexMemoryCitations(text), CreatedAt: r.Timestamp})
		case p.Role == "user" && !codexContext(text):
			emit(harness.Message{ID: id, Role: "user", Text: text, CreatedAt: r.Timestamp})
		}
	case "reasoning":
		var parts []string
		for _, s := range p.Summary {
			parts = append(parts, s.Text)
		}
		if len(parts) > 0 {
			emit(harness.Message{ID: id, Role: "reasoning", Title: "Pensando", Text: strings.Join(parts, "\n"), CreatedAt: r.Timestamp})
		}
	case "function_call", "custom_tool_call":
		args := p.Arguments + p.Input
		detail := kit.ToolDetail(p.Name, json.RawMessage(args))
		if detail == "" {
			detail = kit.Short(args, 300)
		}
		emit(harness.Message{ID: p.CallID, Role: "tool", Title: p.Name, Detail: detail, Status: "completed", CreatedAt: r.Timestamp, Tool: codexTool(p.Name, args)})
	case "function_call_output", "custom_tool_call_output":
		if i, ok := index[p.CallID]; ok {
			m := out[i]
			o := codexOutput(p.Output)
			m.Text = kit.Clip(o, 4000)
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
func (c Codex) SessionFile(p harness.Paths, id string) string {
	return kit.FirstMatch(filepath.Join(kit.AccountDir(p, c), "sessions", "*", "*", "*", "rollout-*"+id+".jsonl"))
}
