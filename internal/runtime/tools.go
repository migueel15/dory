package runtime

import (
	"strings"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
)

type ToolCall = harness.ToolCall
type FileEdit = harness.FileEdit
type Todo = harness.Todo
type Question = harness.Question
type Option = harness.Option

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
	t := kit.ToolCallFromInput(title, nil)
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
