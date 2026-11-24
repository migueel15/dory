package kit

import (
	"encoding/json"
	"testing"
)

func TestToolCallKinds(t *testing.T) {
	cases := map[string]struct {
		name, input, kind string
	}{
		"claude edit":     {"Edit", `{"file_path":"/a/b.go","old_string":"x\ny","new_string":"z"}`, "edit"},
		"opencode edit":   {"edit", `{"filePath":"/a/b.go","oldString":"x","newString":"y"}`, "edit"},
		"bash":            {"Bash", `{"command":"go test ./..."}`, "command"},
		"todo":            {"TodoWrite", `{"todos":[{"content":"a","status":"completed"}]}`, "todo"},
		"read":            {"Read", `{"file_path":"/a","offset":10,"limit":20}`, "read"},
		"grep":            {"Grep", `{"pattern":"foo","path":"src"}`, "search"},
		"fetch":           {"WebFetch", `{"url":"https://go.dev"}`, "web"},
		"task":            {"Task", `{"subagent_type":"Explore","description":"find x"}`, "agent"},
		"mcp":             {"mcp__dory__recall", `{"query":"x"}`, "mcp"},
		"ask":             {"AskUserQuestion", `{"questions":[{"question":"q?","header":"H","options":[{"label":"a"}],"multiSelect":true}]}`, "question"},
		"unknown harness": {"Frobnicate", `{}`, "other"},
	}
	for label, c := range cases {
		got := ToolCallFromInput(c.name, json.RawMessage(c.input))
		if got.Kind != c.kind {
			t.Errorf("%s: kind %q, want %q", label, got.Kind, c.kind)
		}
	}
	e := ToolCallFromInput("Edit", json.RawMessage(`{"file_path":"/a/b.go","old_string":"x\ny","new_string":"z"}`))
	if f := e.Files[0]; f.Added != 1 || f.Removed != 2 || f.Diff != "-x\n-y\n+z" {
		t.Errorf("edit diff: %+v", f)
	}
	r := ToolCallFromInput("Read", json.RawMessage(`{"file_path":"/a","offset":10,"limit":20}`))
	if r.Lines != "10-30" {
		t.Errorf("read lines %q", r.Lines)
	}
	q := ToolCallFromInput("AskUserQuestion", json.RawMessage(`{"questions":[{"question":"q?","options":[{"label":"a"}],"multiSelect":true}]}`))
	if len(q.Questions) != 1 || !q.Questions[0].MultiSelect || q.Questions[0].Options[0].Label != "a" {
		t.Errorf("questions: %+v", q.Questions)
	}
}

func TestPlanAndBashDescription(t *testing.T) {
	if c := ToolCallFromInput("ExitPlanMode", json.RawMessage(`{"plan":"# Plan"}`)); c.Kind != "plan" || c.Summary != "# Plan" {
		t.Fatalf("plan: %+v", c)
	}
	if c := ToolCallFromInput("Bash", json.RawMessage(`{"command":"ls","description":"List files"}`)); c.Summary != "List files" {
		t.Fatalf("bash description: %+v", c)
	}
}
