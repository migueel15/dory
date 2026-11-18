package runtime

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
		got := toolCall(c.name, json.RawMessage(c.input))
		if got.Kind != c.kind {
			t.Errorf("%s: kind %q, want %q", label, got.Kind, c.kind)
		}
	}
	e := toolCall("Edit", json.RawMessage(`{"file_path":"/a/b.go","old_string":"x\ny","new_string":"z"}`))
	if f := e.Files[0]; f.Added != 1 || f.Removed != 2 || f.Diff != "-x\n-y\n+z" {
		t.Errorf("edit diff: %+v", f)
	}
	r := toolCall("Read", json.RawMessage(`{"file_path":"/a","offset":10,"limit":20}`))
	if r.Lines != "10-30" {
		t.Errorf("read lines %q", r.Lines)
	}
	q := toolCall("AskUserQuestion", json.RawMessage(`{"questions":[{"question":"q?","options":[{"label":"a"}],"multiSelect":true}]}`))
	if len(q.Questions) != 1 || !q.Questions[0].MultiSelect || q.Questions[0].Options[0].Label != "a" {
		t.Errorf("questions: %+v", q.Questions)
	}
}

func TestCodexTool(t *testing.T) {
	patch := `{"input":"*** Begin Patch\n*** Update File: a.go\n@@\n-old\n+new\n+more\n*** Add File: b.go\n+hi\n*** End Patch"}`
	got := codexTool("apply_patch", patch)
	if got.Kind != "edit" || len(got.Files) != 2 || got.Files[0].Added != 2 || got.Files[0].Removed != 1 || got.Files[1].Change != "add" {
		t.Fatalf("patch: %+v", got.Files)
	}
	cmd := codexTool("shell", `{"command":["bash","-lc","ls -la"]}`)
	if cmd.Kind != "command" || cmd.Command != "ls -la" {
		t.Fatalf("shell: %+v", cmd)
	}
	plan := codexTool("update_plan", `{"plan":[{"step":"a","status":"in_progress"}]}`)
	if plan.Kind != "todo" || plan.Todos[0].Status != "in_progress" {
		t.Fatalf("plan: %+v", plan)
	}
}

func TestLegacyAndPlan(t *testing.T) {
	if c := legacyTool("Bash", "go test ./..."); c.Kind != "command" || c.Command != "go test ./..." {
		t.Fatalf("bash: %+v", c)
	}
	if c := legacyTool("Editar ficheros", "update a.go\nadd b.go"); len(c.Files) != 2 || c.Files[1].Change != "add" {
		t.Fatalf("codex files: %+v", c)
	}
	if c := legacyTool("Read", "/a/b.go"); c.Kind != "read" || c.Path != "/a/b.go" {
		t.Fatalf("read: %+v", c)
	}
	if c := toolCall("ExitPlanMode", json.RawMessage(`{"plan":"# Plan"}`)); c.Kind != "plan" || c.Summary != "# Plan" {
		t.Fatalf("plan: %+v", c)
	}
	if c := toolCall("Bash", json.RawMessage(`{"command":"ls","description":"List files"}`)); c.Summary != "List files" {
		t.Fatalf("bash description: %+v", c)
	}
}
