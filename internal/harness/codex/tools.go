package codex

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
)

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

func codexTool(name, args string) *harness.ToolCall {
	switch name {
	case "apply_patch":
		patch := args
		var in struct {
			Input string `json:"input"`
		}
		if json.Unmarshal([]byte(args), &in) == nil && in.Input != "" {
			patch = in.Input
		}
		return &harness.ToolCall{Kind: "edit", Name: name, Files: patchFiles(patch)}
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
		return &harness.ToolCall{Kind: "command", Name: name, Command: cmd}
	case "update_plan":
		var in struct {
			Plan []struct {
				Step   string `json:"step"`
				Status string `json:"status"`
			} `json:"plan"`
		}
		_ = json.Unmarshal([]byte(args), &in)
		t := &harness.ToolCall{Kind: "todo", Name: name}
		for _, x := range in.Plan {
			t.Todos = append(t.Todos, harness.Todo{Text: x.Step, Status: x.Status})
		}
		return t
	case "request_user_input":
		return &harness.ToolCall{Kind: "question", Name: name, Questions: kit.ParseQuestions(json.RawMessage(args))}
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
				return &harness.ToolCall{Kind: "edit", Name: call, Files: patchFiles(patch)}
			}
		}
		if call != "" {
			t := kit.ToolCallFromInput(call, nil)
			if t.Kind == "mcp" || t.Kind == "other" {
				t.Input = kit.Clip(args, 4000)
				return t
			}
		}
	}
	return kit.ToolCallFromInput(name, json.RawMessage(args))
}

// codexAnswers sets on a copy of the questions the answers of a
// request_user_input result: {"answers": {"<question id>": {"answers": [...]}}}.
func codexAnswers(qs []harness.Question, output string) ([]harness.Question, bool) {
	var res struct {
		Answers map[string]struct {
			Answers []string `json:"answers"`
		} `json:"answers"`
	}
	if json.Unmarshal([]byte(output), &res) != nil || len(res.Answers) == 0 {
		return qs, false
	}
	out := append([]harness.Question(nil), qs...)
	for i, q := range out {
		if a, ok := res.Answers[q.ID]; ok {
			out[i].Answer = a.Answers
		}
	}
	return out, true
}

var codeModeCall = regexp.MustCompile(`\btools\.(\w+)\s*\(`)
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
func patchFiles(patch string) []harness.FileEdit {
	var files []harness.FileEdit
	var cur *harness.FileEdit
	var body []string
	flush := func() {
		if cur != nil {
			cur.Diff = kit.Clip(strings.Join(body, "\n"), 12000)
			files = append(files, *cur)
		}
		body = nil
	}
	for _, l := range strings.Split(patch, "\n") {
		for prefix, change := range map[string]string{"*** Add File: ": "add", "*** Update File: ": "update", "*** Delete File: ": "delete"} {
			if strings.HasPrefix(l, prefix) {
				flush()
				cur = &harness.FileEdit{Path: strings.TrimPrefix(l, prefix), Change: change}
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
