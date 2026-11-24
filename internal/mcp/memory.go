package mcp

import (
	"context"

	"github.com/migueel15/dory/internal/app"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type memoryInput struct {
	ID      string   `json:"id,omitempty" jsonschema:"Set to update an existing memory"`
	Scope   string   `json:"scope,omitempty" jsonschema:"global, project or node"`
	ScopeID string   `json:"scope_id,omitempty" jsonschema:"Project id or node id; empty for global"`
	Kind    string   `json:"kind,omitempty" jsonschema:"decision, lesson or context"`
	Title   string   `json:"title,omitempty"`
	Content string   `json:"content,omitempty"`
	Labels  []string `json:"labels,omitempty"`
}

type recallInput struct {
	ID        string   `json:"id,omitempty" jsonschema:"Read one memory by id; the other fields are ignored"`
	Query     string   `json:"query,omitempty" jsonschema:"Words to search; empty lists by filters"`
	ProjectID string   `json:"project_id,omitempty" jsonschema:"Only the memories of this project"`
	Labels    []string `json:"labels,omitempty" jsonschema:"Memories that carry all these labels"`
	Kind      string   `json:"kind,omitempty"`
	Limit     int      `json:"limit,omitempty" jsonschema:"Most memories to return; default 8, best match first"`
}

func addMemoryTools(srv *sdk.Server, s *Deps) {
	add(srv, "remember", "Save a decision the user agreed or a verified lesson the code does not tell. Scope defaults to this thread's project; a memory with the same title in that scope is updated instead of duplicated.", func(ctx context.Context, in memoryInput) (any, error) {
		if in.ID == "" && in.Scope == "" {
			thread, _ := ctx.Value(threadKey{}).(string)
			in.Scope, in.ScopeID = s.Memories.ThreadScope(ctx, thread)
		}
		return s.Memories.Remember(ctx, app.RememberInput{ID: in.ID, Scope: in.Scope, ScopeID: in.ScopeID, Kind: in.Kind, Title: in.Title, Content: in.Content, Labels: in.Labels})
	})
	add(srv, "recall", "Find a few memories by text, project, labels or kind, best match first, or read one by id.", func(ctx context.Context, in recallInput) (any, error) {
		return s.Memories.Recall(ctx, app.RecallInput{ID: in.ID, Query: in.Query, ProjectID: in.ProjectID, Labels: in.Labels, Kind: in.Kind, Limit: in.Limit})
	})
	add(srv, "search", "Search tasks and memories by text.", func(ctx context.Context, in struct {
		Query string `json:"query"`
	}) (any, error) {
		return s.Store.Search(ctx, in.Query, 20)
	})
}
