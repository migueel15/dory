package localdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
)

func migrateMessageTools(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT seq, title, detail FROM thread_messages WHERE role = 'tool' AND tool = ''`)
	if err != nil {
		return err
	}
	type update struct {
		seq  int64
		tool string
	}
	var updates []update
	for rows.Next() {
		var seq int64
		var title, detail string
		if err := rows.Scan(&seq, &title, &detail); err != nil {
			rows.Close()
			return err
		}
		if tool := decodeOldTool(title, detail); tool != nil {
			encoded, err := json.Marshal(tool)
			if err != nil {
				rows.Close()
				return err
			}
			updates = append(updates, update{seq, string(encoded)})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range updates {
		if _, err := tx.ExecContext(ctx, `UPDATE thread_messages SET tool = ? WHERE seq = ?`, item.tool, item.seq); err != nil {
			return err
		}
	}
	return nil
}

func decodeOldTool(title, detail string) *harness.ToolCall {
	switch title {
	case "Bash", "bash", "shell", "exec_command", "command execution":
		return &harness.ToolCall{Kind: "command", Name: title, Command: detail}
	case "Editar ficheros":
		t := &harness.ToolCall{Kind: "edit", Name: "file_change"}
		for _, l := range strings.Split(detail, "\n") {
			if change, path, ok := strings.Cut(l, " "); ok {
				t.Files = append(t.Files, harness.FileEdit{Path: path, Change: change})
			}
		}
		return t
	case "Buscar en la web":
		return &harness.ToolCall{Kind: "web", Name: "web_search", Query: detail}
	}
	t := kit.ToolCallFromInput(title, nil)
	switch t.Kind {
	case "edit":
		t.Files = []harness.FileEdit{{Path: detail, Change: "update"}}
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
