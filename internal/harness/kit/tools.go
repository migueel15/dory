package kit

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/migueel15/dory/internal/harness"
)

// toolCall maps a call by name and JSON arguments; Claude Code and OpenCode
// name their tools alike (Edit/edit, Bash/bash…), so both use it.
func ToolCallFromInput(name string, input json.RawMessage) *harness.ToolCall {
	var a map[string]any
	_ = json.Unmarshal(input, &a)
	str := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := a[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	t := &harness.ToolCall{Name: name}
	switch strings.ToLower(name) {
	case "edit", "multiedit", "write", "notebookedit", "patch":
		t.Kind = "edit"
		path := str("file_path", "filePath", "notebook_path", "path")
		switch strings.ToLower(name) {
		case "write":
			t.Files = []harness.FileEdit{LineDiff(path, "", str("content"))}
		case "multiedit":
			f := harness.FileEdit{Path: path, Change: "update"}
			if edits, ok := a["edits"].([]any); ok {
				for _, e := range edits {
					if m, ok := e.(map[string]any); ok {
						old, _ := m["old_string"].(string)
						nw, _ := m["new_string"].(string)
						d := LineDiff(path, old, nw)
						f.Added, f.Removed = f.Added+d.Added, f.Removed+d.Removed
						f.Diff = strings.TrimPrefix(f.Diff+"\n"+d.Diff, "\n")
					}
				}
			}
			t.Files = []harness.FileEdit{f}
		default:
			t.Files = []harness.FileEdit{LineDiff(path, str("old_string", "oldString"), str("new_string", "newString", "new_source"))}
		}
	case "bash", "shell", "exec_command":
		t.Kind, t.Command, t.Summary = "command", str("command", "cmd"), str("description")
	case "todowrite":
		t.Kind = "todo"
		if todos, ok := a["todos"].([]any); ok {
			for _, x := range todos {
				if m, ok := x.(map[string]any); ok {
					text, _ := m["content"].(string)
					status, _ := m["status"].(string)
					active, _ := m["activeForm"].(string)
					t.Todos = append(t.Todos, harness.Todo{Text: text, Status: status, Active: active})
				}
			}
		}
	case "read", "view":
		t.Kind, t.Path = "read", str("file_path", "filePath", "path")
		if off, ok := a["offset"].(float64); ok {
			t.Lines = strconv.Itoa(int(off))
			if lim, ok := a["limit"].(float64); ok {
				t.Lines += "-" + strconv.Itoa(int(off+lim))
			}
		}
	case "grep", "glob", "list", "ls":
		t.Kind, t.Pattern, t.Path = "search", str("pattern", "glob"), str("path")
	case "webfetch", "websearch", "web_search", "fetch":
		t.Kind, t.URL, t.Query = "web", str("url"), str("query")
	case "task", "agent":
		t.Kind, t.Agent, t.Summary = "agent", str("subagent_type"), str("description", "prompt")
	case "exitplanmode", "plan_exit":
		// The plan the agent proposes before acting; the chat shows it as a document.
		t.Kind, t.Summary = "plan", str("plan")
	case "askuserquestion", "question":
		t.Kind = "question"
		t.Questions = ParseQuestions(input)
	default:
		if server, tool, ok := McpName(name); ok {
			t.Kind, t.Server, t.Tool = "mcp", server, tool
		} else {
			t.Kind = "other"
		}
		if len(input) > 0 && string(input) != "null" {
			t.Input = Clip(string(input), 4000)
		}
	}
	return t
}

var codeModeCall = regexp.MustCompile(`\btools\.(\w+)\s*\(`)

// codexOutput reads a tool's output from a rollout: a string, or a list of
// text parts. A code-mode script returns its tool's MCP result as one part
// ({"content":[{"type":"text","text":...}]}); then only that result counts.

func McpName(name string) (server, tool string, ok bool) {
	if !strings.HasPrefix(name, "mcp__") {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(name, "mcp__"), "__", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// parseQuestions reads AskUserQuestion's arguments (or ask_user's).
func ParseQuestions(input json.RawMessage) []harness.Question {
	var in struct {
		Questions []struct {
			ID          string           `json:"id"`
			Question    string           `json:"question"`
			Header      string           `json:"header"`
			Options     []harness.Option `json:"options"`
			MultiSelect bool             `json:"multiSelect"`
			Multi       bool             `json:"multi_select"`
		} `json:"questions"`
	}
	_ = json.Unmarshal(input, &in)
	var out []harness.Question
	for _, q := range in.Questions {
		out = append(out, harness.Question{ID: q.ID, Question: q.Question, Header: q.Header, Options: q.Options, MultiSelect: q.MultiSelect || q.Multi})
	}
	return out
}

// lineDiff is a whole-block diff of one replacement: the old lines out, the
// new ones in. It is what the harness reports, not a computed minimal diff.
func LineDiff(path, old, nw string) harness.FileEdit {
	f := harness.FileEdit{Path: path, Change: "update"}
	if old == "" {
		f.Change = "add"
	}
	var b strings.Builder
	write := func(mark string, s string) int {
		if s == "" {
			return 0
		}
		lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
		for _, l := range lines {
			b.WriteString(mark + l + "\n")
		}
		return len(lines)
	}
	f.Removed = write("-", old)
	f.Added = write("+", nw)
	f.Diff = Clip(strings.TrimSuffix(b.String(), "\n"), 12000)
	return f
}

// toolDetail picks the most telling argument of a tool call.
func ToolDetail(name string, input json.RawMessage) string {
	var args map[string]any
	if json.Unmarshal(input, &args) != nil {
		return ""
	}
	for _, key := range []string{"command", "file_path", "path", "pattern", "url", "query", "description", "prompt"} {
		if v, ok := args[key].(string); ok && v != "" {
			return Short(v, 300)
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
func ResultText(raw json.RawMessage) string {
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

func CopyOf(m *harness.Message) *harness.Message {
	c := *m
	if m.Tool != nil {
		t := *m.Tool
		t.Questions = append([]harness.Question(nil), t.Questions...)
		c.Tool = &t
	}
	return &c
}

// withAnswers sets the answers Claude reports, keyed by question text, on a
// copy of the questions. Several choices come joined by ", ".

func Answered(qs []harness.Question, answers [][]string) []harness.Question {
	out := append([]harness.Question(nil), qs...)
	for i := range out {
		if i < len(answers) {
			out[i].Answer = answers[i]
		}
	}
	return out
}

// withDiff counts the lines of a unified diff into a file edit.
