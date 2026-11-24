package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type planReadInput struct {
	TaskID string `json:"task_id"`
}

type planWriteInput struct {
	TaskID   string `json:"task_id"`
	BodyMD   string `json:"body_md" jsonschema:"The whole plan as Markdown"`
	Revision *int64 `json:"revision,omitempty" jsonschema:"Revision returned by plan_read; the save fails if the plan changed since"`
}

type planEditInput struct {
	TaskID   string `json:"task_id"`
	OldText  string `json:"old_text" jsonschema:"Exact text of the plan to replace; it must appear once"`
	NewText  string `json:"new_text"`
	Revision *int64 `json:"revision,omitempty" jsonschema:"Revision returned by plan_read; the edit fails if the plan changed since"`
}

func addPlanTools(srv *sdk.Server, s *Deps) {
	add(srv, "plan_read", "Read a task's plan (Markdown) and its revision.", func(ctx context.Context, in planReadInput) (any, error) {
		return s.Plans.Document(ctx, in.TaskID)
	})
	add(srv, "plan_write", "Write a task's whole plan. For an existing plan use plan_edit, or pass the revision from plan_read. Only the user accepts it.", func(ctx context.Context, in planWriteInput) (any, error) {
		return s.Plans.WriteByAgent(ctx, in.TaskID, in.BodyMD, in.Revision)
	})
	add(srv, "plan_edit", "Replace one exact piece of a task's plan; the user sees it change in place. old_text must appear once.", func(ctx context.Context, in planEditInput) (any, error) {
		return s.Plans.EditByAgent(ctx, in.TaskID, in.OldText, in.NewText, in.Revision)
	})
}
