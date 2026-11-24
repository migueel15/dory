package runtime

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/migueel15/dory/internal/harness"
)

type ToolCall = harness.ToolCall

// FileEdit is one file an edit touches; Diff holds unified-diff body lines
// ("+", "-" or " " first) when the harness reports the content.
type FileEdit = harness.FileEdit

type Todo = harness.Todo

type Question = harness.Question

type Option = harness.Option

// toolCall maps a call by name and JSON arguments; Claude Code and OpenCode
// name their tools alike (Edit/edit, Bash/bash…), so both use it.
func toolCall(name string, input json.RawMessage) *ToolCall {
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
	t := &ToolCall{Name: name}
	switch strings.ToLower(name) {
	case "edit", "multiedit", "write", "notebookedit", "patch":
		t.Kind = "edit"
		path := str("file_path", "filePath", "notebook_path", "path")
		switch strings.ToLower(name) {
		case "write":
			t.Files = []FileEdit{lineDiff(path, "", str("content"))}
		case "multiedit":
			f := FileEdit{Path: path, Change: "update"}
			if edits, ok := a["edits"].([]any); ok {
				for _, e := range edits {
					if m, ok := e.(map[string]any); ok {
						old, _ := m["old_string"].(string)
						nw, _ := m["new_string"].(string)
						d := lineDiff(path, old, nw)
						f.Added, f.Removed = f.Added+d.Added, f.Removed+d.Removed
						f.Diff = strings.TrimPrefix(f.Diff+"\n"+d.Diff, "\n")
					}
				}
			}
			t.Files = []FileEdit{f}
		default:
			t.Files = []FileEdit{lineDiff(path, str("old_string", "oldString"), str("new_string", "newString", "new_source"))}
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
					t.Todos = append(t.Todos, Todo{Text: text, Status: status, Active: active})
				}
			}
		}
	case "read", "view":
		t.Kind, t.Path = "read", str("file_path", "filePath", "path")
		if off, ok := a["offset"].(float64); ok {
			t.Lines = itoa(int(off))
			if lim, ok := a["limit"].(float64); ok {
				t.Lines += "-" + itoa(int(off+lim))
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
		t.Questions = parseQuestions(input)
	default:
		if server, tool, ok := mcpName(name); ok {
			t.Kind, t.Server, t.Tool = "mcp", server, tool
		} else {
			t.Kind = "other"
		}
		if len(input) > 0 && string(input) != "null" {
			t.Input = clip(string(input), 4000)
		}
	}
	return t
}

var codeModeCall = regexp.MustCompile(`\btools\.(\w+)\s*\(`)

// codexOutput reads a tool's output from a rollout: a string, or a list of
// text parts. A code-mode script returns its tool's MCP result as one part
// ({"content":[{"type":"text","text":...}]}); then only that result counts.
func codexOutput(o any) string {
	switch o := o.(type) {
	case string:
		return o
	case []any:
		var all, inner []string
		for _, x := range o {
			part, _ := x.(map[string]any)
			text, _ := part["text"].(string)
			all = append(all, text)
			var res struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			}
			if json.Unmarshal([]byte(text), &res) == nil {
				for _, c := range res.Content {
					if c.Type == "text" {
						inner = append(inner, c.Text)
					}
				}
			}
		}
		if len(inner) > 0 {
			return strings.Join(inner, "\n")
		}
		return strings.Join(all, "\n")
	}
	raw, _ := json.Marshal(o)
	return string(raw)
}

// mcpName splits "mcp__server__tool" (Claude) or "server_tool" is left alone.
func mcpName(name string) (server, tool string, ok bool) {
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
func parseQuestions(input json.RawMessage) []Question {
	var in struct {
		Questions []struct {
			ID          string   `json:"id"`
			Question    string   `json:"question"`
			Header      string   `json:"header"`
			Options     []Option `json:"options"`
			MultiSelect bool     `json:"multiSelect"`
			Multi       bool     `json:"multi_select"`
		} `json:"questions"`
	}
	_ = json.Unmarshal(input, &in)
	var out []Question
	for _, q := range in.Questions {
		out = append(out, Question{ID: q.ID, Question: q.Question, Header: q.Header, Options: q.Options, MultiSelect: q.MultiSelect || q.Multi})
	}
	return out
}

// lineDiff is a whole-block diff of one replacement: the old lines out, the
// new ones in. It is what the harness reports, not a computed minimal diff.
func lineDiff(path, old, nw string) FileEdit {
	f := FileEdit{Path: path, Change: "update"}
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
	f.Diff = clip(strings.TrimSuffix(b.String(), "\n"), 12000)
	return f
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// codexTool maps the function calls Codex saves in its session files.
func codexTool(name, args string) *ToolCall {
	switch name {
	case "apply_patch":
		patch := args
		var in struct {
			Input string `json:"input"`
		}
		if json.Unmarshal([]byte(args), &in) == nil && in.Input != "" {
			patch = in.Input
		}
		return &ToolCall{Kind: "edit", Name: name, Files: patchFiles(patch)}
	case "shell", "exec_command", "local_shell":
		var in struct {
			Command any    `json:"command"`
			Cmd     string `json:"cmd"`
		}
		_ = json.Unmarshal([]byte(args), &in)
		cmd := in.Cmd
		switch c := in.Command.(type) {
		case string:
			cmd = c
		case []any:
			var parts []string
			for _, x := range c {
				parts = append(parts, fmt.Sprint(x))
			}
			// ["bash", "-lc", "real command"] shows the real command.
			if len(parts) == 3 && parts[1] == "-lc" {
				cmd = parts[2]
			} else {
				cmd = strings.Join(parts, " ")
			}
		}
		return &ToolCall{Kind: "command", Name: name, Command: cmd}
	case "update_plan":
		var in struct {
			Plan []struct {
				Step   string `json:"step"`
				Status string `json:"status"`
			} `json:"plan"`
		}
		_ = json.Unmarshal([]byte(args), &in)
		t := &ToolCall{Kind: "todo", Name: name}
		for _, x := range in.Plan {
			t.Todos = append(t.Todos, Todo{Text: x.Step, Status: x.Status})
		}
		return t
	case "request_user_input":
		return &ToolCall{Kind: "question", Name: name, Questions: parseQuestions(json.RawMessage(args))}
	case "exec":
		// Code mode: a script that calls tools as tools.<name>(...). The card
		// shows the tool it calls, with the script as its input.
		// A page it renders is what the chat shows, so render_html wins.
		var call string
		for _, c := range codeModeCall.FindAllStringSubmatch(args, -1) {
			if call == "" || strings.HasSuffix(c[1], "render_html") {
				call = c[1]
			}
		}
		if call == "apply_patch" {
			if patch := scriptPatch(args); patch != "" {
				return &ToolCall{Kind: "edit", Name: call, Files: patchFiles(patch)}
			}
		}
		if call != "" {
			t := toolCall(call, nil)
			if t.Kind == "mcp" || t.Kind == "other" {
				t.Input = clip(args, 4000)
				return t
			}
		}
	}
	return toolCall(name, json.RawMessage(args))
}

// codexAnswers sets on a copy of the questions the answers of a
// request_user_input result: {"answers": {"<question id>": {"answers": [...]}}}.
func codexAnswers(qs []Question, output string) ([]Question, bool) {
	var res struct {
		Answers map[string]struct {
			Answers []string `json:"answers"`
		} `json:"answers"`
	}
	if json.Unmarshal([]byte(output), &res) != nil || len(res.Answers) == 0 {
		return qs, false
	}
	out := append([]Question(nil), qs...)
	for i, q := range out {
		if a, ok := res.Answers[q.ID]; ok {
			out[i].Answer = a.Answers
		}
	}
	return out, true
}

var scriptPatchLit = regexp.MustCompile(`"\*\*\* Begin Patch(?:[^"\\]|\\.)*"|'\*\*\* Begin Patch(?:[^'\\]|\\.)*'|` + "`" + `\*\*\* Begin Patch[^` + "`" + `]*` + "`")

// scriptPatch reads the apply_patch envelope a code-mode script holds as a
// JavaScript string literal, or "" if it has none.
func scriptPatch(script string) string {
	var in struct {
		Input string `json:"input"`
	}
	if json.Unmarshal([]byte(script), &in) == nil && in.Input != "" {
		script = in.Input
	}
	lit := scriptPatchLit.FindString(script)
	if lit == "" {
		return ""
	}
	body := lit[1 : len(lit)-1]
	if lit[0] == '`' {
		return body
	}
	if lit[0] == '\'' {
		body = strings.ReplaceAll(strings.ReplaceAll(body, `\'`, `'`), `"`, `\"`)
	}
	var out string
	if json.Unmarshal([]byte(`"`+body+`"`), &out) != nil {
		return ""
	}
	return out
}

// patchFiles reads the files and line changes of an apply_patch envelope.
func patchFiles(patch string) []FileEdit {
	var files []FileEdit
	var cur *FileEdit
	var body []string
	flush := func() {
		if cur != nil {
			cur.Diff = clip(strings.Join(body, "\n"), 12000)
			files = append(files, *cur)
		}
		body = nil
	}
	for _, l := range strings.Split(patch, "\n") {
		for prefix, change := range map[string]string{"*** Add File: ": "add", "*** Update File: ": "update", "*** Delete File: ": "delete"} {
			if strings.HasPrefix(l, prefix) {
				flush()
				cur = &FileEdit{Path: strings.TrimPrefix(l, prefix), Change: change}
			}
		}
		if cur == nil || strings.HasPrefix(l, "***") || strings.HasPrefix(l, "@@") {
			continue
		}
		switch {
		case strings.HasPrefix(l, "+"):
			cur.Added++
			body = append(body, l)
		case strings.HasPrefix(l, "-"):
			cur.Removed++
			body = append(body, l)
		case strings.HasPrefix(l, " "):
			body = append(body, l)
		}
	}
	flush()
	return files
}

// legacyTool rebuilds the call of a message saved before calls carried their
// Dory kind, from the title (tool name) and detail (main argument) it kept.
func legacyTool(title, detail string) *ToolCall {
	switch title {
	case "Bash", "bash", "shell", "exec_command", "command execution":
		return &ToolCall{Kind: "command", Name: title, Command: detail}
	case "Editar ficheros":
		t := &ToolCall{Kind: "edit", Name: "file_change"}
		for _, l := range strings.Split(detail, "\n") {
			if change, path, ok := strings.Cut(l, " "); ok {
				t.Files = append(t.Files, FileEdit{Path: path, Change: change})
			}
		}
		return t
	case "Buscar en la web":
		return &ToolCall{Kind: "web", Name: "web_search", Query: detail}
	}
	t := toolCall(title, nil)
	switch t.Kind {
	case "edit":
		t.Files = []FileEdit{{Path: detail, Change: "update"}}
	case "read":
		t.Path = detail
	case "search":
		t.Pattern = detail
	case "web":
		if strings.HasPrefix(detail, "http") {
			t.URL = detail
		} else {
			t.Query = detail
		}
	case "agent":
		t.Summary = detail
	case "todo", "plan", "question":
		return nil // their old cards (plan text) still read well
	case "other", "mcp":
		t.Input = ""
		if detail != "" {
			t.Input = detail
		}
	}
	return t
}
