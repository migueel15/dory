// Package mcp exposes dory tools over streamable HTTP. The caller must wrap
// Handler with bearer authentication before mounting it at /mcp.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	"github.com/migueel15/dory/internal/service"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type idInput struct {
	ID string `json:"id" jsonschema:"Resource UUID"`
}
type listInput struct {
	Fields          map[string]any `json:"fields,omitempty" jsonschema:"Exact field filters; tags use array containment"`
	Limit           int32          `json:"limit,omitempty" jsonschema:"Page size; default 100 maximum 1000"`
	Offset          int32          `json:"offset,omitempty"`
	IncludeArchived bool           `json:"include_archived,omitempty"`
}

func (in listInput) filter() service.Filter {
	return service.Filter{Fields: in.Fields, Limit: in.Limit, Offset: in.Offset, IncludeArchived: in.IncludeArchived}
}

// add uses the SDK's inferred input schema and reports service failures as tool
// errors. Results include JSON text for clients that do not read structured data.
func add[I any](server *sdk.Server, name, description string, fn func(context.Context, I) (any, error)) {
	sdk.AddTool(server, &sdk.Tool{Name: name, Description: description}, func(ctx context.Context, _ *sdk.CallToolRequest, in I) (*sdk.CallToolResult, any, error) {
		value, err := fn(ctx, in)
		if err != nil {
			return nil, nil, err
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, nil, err
		}
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(raw)}}, StructuredContent: map[string]any{"result": value}}, nil, nil
	})
}
func listTool[T any](s *sdk.Server, name, description string, r service.Resource[T]) {
	add(s, name, description, func(ctx context.Context, in listInput) (any, error) { return r.List(ctx, in.filter()) })
}
func getTool[T any](s *sdk.Server, name, description string, r service.Resource[T]) {
	add(s, name, description, func(ctx context.Context, in idInput) (any, error) { return r.Get(ctx, in.ID) })
}

type projectInput struct {
	Project string `json:"project,omitempty" jsonschema:"Optional project UUID or slug"`
}
type projectDetail struct {
	Project      service.Project      `json:"project"`
	Repositories []service.Repository `json:"repositories"`
	Features     []service.Feature    `json:"features"`
	Memories     []service.Memory     `json:"memories"`
	OpenItems    []service.Item       `json:"open_items"`
}

func projectContext(ctx context.Context, svc service.Service, key string) (projectDetail, error) {
	var result projectDetail
	// Resolve slugs with a list filter rather than sending them to a UUID lookup.
	projects, err := svc.Projects().List(ctx, service.Filter{Fields: map[string]any{"slug": key}, Limit: 1})
	if err != nil {
		return result, err
	}
	if len(projects) > 0 {
		result.Project = projects[0]
	} else {
		result.Project, err = svc.Projects().Get(ctx, key)
		if err != nil {
			return result, err
		}
	}
	f := service.Filter{Fields: map[string]any{"project_id": result.Project.ID}, Limit: 1000}
	// Read each collection in pages so context does not silently omit open tasks.
	result.Repositories, err = all(ctx, svc.Repositories(), f)
	if err != nil {
		return result, err
	}
	result.Features, err = all(ctx, svc.Features(), f)
	if err != nil {
		return result, err
	}
	result.Memories, err = all(ctx, svc.Memories(), service.Filter{Fields: map[string]any{"scope": "project", "scope_id": result.Project.ID}, Limit: 1000})
	if err != nil {
		return result, err
	}
	items, err := all(ctx, svc.Items(), f)
	result.OpenItems = []service.Item{}
	for _, item := range items {
		if item.Status != "done" && item.Status != "archived" {
			result.OpenItems = append(result.OpenItems, item)
		}
	}
	return result, err
}
func all[T any](ctx context.Context, r service.Resource[T], f service.Filter) ([]T, error) {
	result := []T{}
	f.Limit = 1000
	for {
		rows, err := r.List(ctx, f)
		if err != nil {
			return nil, err
		}
		result = append(result, rows...)
		if len(rows) < int(f.Limit) {
			return result, nil
		}
		f.Offset += f.Limit
	}
}

type itemInput struct {
	ID        string   `json:"id,omitempty" jsonschema:"Optional client UUID for idempotent upsert"`
	ProjectID *string  `json:"project_id,omitempty"`
	RawText   string   `json:"raw_text,omitempty"`
	Title     string   `json:"title"`
	BodyMd    string   `json:"body_md,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Status    string   `json:"status,omitempty"`
	Priority  int32    `json:"priority,omitempty"`
	Source    string   `json:"source,omitempty"`
}
type updateInput struct {
	ID      string         `json:"id"`
	Changes map[string]any `json:"changes" jsonschema:"Patch fields: project_id (nullable), raw_text, title, body_md, tags, status, priority, source. Omitted fields are preserved."`
}
type moveInput struct {
	ID     string `json:"id"`
	Status string `json:"status" jsonschema:"inbox, triaged, ready, in_progress, done or archived"`
}

func validStatus(status string) bool {
	return slices.Contains([]string{"inbox", "triaged", "ready", "in_progress", "done", "archived"}, status)
}
func putItem(ctx context.Context, svc service.Service, item service.Item) (any, error) {
	if item.Status != "" && !validStatus(item.Status) {
		return nil, fmt.Errorf("%w: unknown item status", service.ErrInvalid)
	}
	return svc.Items().Put(ctx, "mcp", item)
}
func updateItem(ctx context.Context, svc service.Service, in updateInput) (any, error) {
	item, err := svc.Items().Get(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	for key, value := range in.Changes {
		if !slices.Contains([]string{"project_id", "raw_text", "title", "body_md", "tags", "status", "priority", "source"}, key) {
			return nil, fmt.Errorf("%w: field %s cannot be changed", service.ErrInvalid, key)
		}
		if value == nil && key != "project_id" {
			return nil, fmt.Errorf("%w: only project_id may be null", service.ErrInvalid)
		}
		fields[key], err = json.Marshal(value)
		if err != nil {
			return nil, err
		}
	}
	raw, err = json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &item); err != nil {
		return nil, fmt.Errorf("%w: %v", service.ErrInvalid, err)
	}
	return putItem(ctx, svc, item)
}

type featureInput struct {
	ID        string   `json:"id,omitempty"`
	ProjectID string   `json:"project_id"`
	Title     string   `json:"title"`
	BodyMd    string   `json:"body_md,omitempty"`
	CodeRefs  []string `json:"code_refs,omitempty"`
	Status    string   `json:"status,omitempty" jsonschema:"implemented (default) or deprecated"`
}
type memoryInput struct {
	Scope   string   `json:"scope" jsonschema:"global, project or environment"`
	ScopeID *string  `json:"scope_id,omitempty" jsonschema:"Required UUID for project or environment scope"`
	Content string   `json:"content"`
	Tags    []string `json:"tags,omitempty"`
}
type recallInput struct {
	Query   string `json:"query"`
	Scope   string `json:"scope,omitempty" jsonschema:"Optional global, project or environment scope"`
	ScopeID string `json:"scope_id,omitempty"`
	Limit   int32  `json:"limit,omitempty" jsonschema:"Default 100 maximum 1000"`
	Offset  int32  `json:"offset,omitempty"`
}

func recall(ctx context.Context, svc service.Service, in recallInput) (any, error) {
	if in.Query == "" || in.Offset < 0 || in.Limit < 0 || in.Limit > 1000 {
		return nil, service.ErrInvalid
	}
	if in.Scope != "" && !slices.Contains([]string{"global", "project", "environment"}, in.Scope) {
		return nil, service.ErrInvalid
	}
	limit := in.Limit
	if limit == 0 {
		limit = 100
	}
	result := []service.SearchHit{}
	// Scope filtering precedes pagination: Search covers items/features too, so
	// filtering a single page could incorrectly hide matching memories.
	var skipped int32
	for offset := int32(0); ; offset += 1000 {
		hits, err := svc.Search(ctx, in.Query, 1000, offset)
		if err != nil {
			return nil, err
		}
		for _, hit := range hits {
			if hit.EntityType != "memory" {
				continue
			}
			var memory service.Memory
			if err = json.Unmarshal(hit.Content, &memory); err != nil {
				return nil, err
			}
			if in.Scope != "" && memory.Scope != in.Scope {
				continue
			}
			if in.ScopeID != "" && (memory.ScopeID == nil || *memory.ScopeID != in.ScopeID) {
				continue
			}
			if skipped < in.Offset {
				skipped++
				continue
			}
			result = append(result, hit)
			if len(result) == int(limit) {
				return result, nil
			}
		}
		if len(hits) < 1000 {
			return result, nil
		}
	}
}

type linkInput struct {
	FromID   string `json:"from_id"`
	FromType string `json:"from_type"`
	ToID     string `json:"to_id"`
	ToType   string `json:"to_type"`
	Kind     string `json:"kind"`
}

// Handler returns the transport mounted by cmd/dory at /mcp. Stateless requests
// avoid keeping authentication-independent sessions between HTTP requests.
func Handler(svc service.Service) http.Handler {
	s := sdk.NewServer(&sdk.Implementation{Name: "dory", Version: "0.1.0"}, nil)
	listTool(s, "list_projects", "List projects with optional field filters and pagination.", svc.Projects())
	listTool(s, "list_items", "List items with optional field filters and pagination.", svc.Items())
	getTool(s, "get_item", "Get an unarchived item by UUID.", svc.Items())
	listTool(s, "list_features", "List implemented or deprecated features with optional filters.", svc.Features())
	listTool(s, "list_environments", "List environments with optional filters.", svc.Environments())
	getTool(s, "get_environment", "Get an environment by UUID.", svc.Environments())
	listTool(s, "list_repositories", "List repositories; filter by project_id if needed.", svc.Repositories())
	add(s, "get_project", "Get project by UUID or slug with repositories, features, project memories and open items.", func(ctx context.Context, in struct {
		ID string `json:"id" jsonschema:"Project UUID or slug"`
	}) (any, error) { return projectContext(ctx, svc, in.ID) })
	add(s, "session_start", "Load global memories and available projects; optionally include project context by UUID or slug.", func(ctx context.Context, in projectInput) (any, error) {
		memories, err := all(ctx, svc.Memories(), service.Filter{Fields: map[string]any{"scope": "global"}})
		if err != nil {
			return nil, err
		}
		projects, err := all(ctx, svc.Projects(), service.Filter{})
		if err != nil {
			return nil, err
		}
		result := map[string]any{"memories": memories, "projects": projects}
		if in.Project != "" {
			detail, err := projectContext(ctx, svc, in.Project)
			if err != nil {
				return nil, err
			}
			result["project"] = detail
		}
		return result, nil
	})
	add(s, "create_item", "Create or idempotently upsert an item; defaults to inbox and source mcp.", func(ctx context.Context, in itemInput) (any, error) {
		if in.Source == "" {
			in.Source = "mcp"
		}
		return putItem(ctx, svc, service.Item{Record: service.Record{ID: in.ID}, ProjectID: in.ProjectID, RawText: in.RawText, Title: in.Title, BodyMd: in.BodyMd, Tags: in.Tags, Status: in.Status, Priority: in.Priority, Source: in.Source})
	})
	add(s, "update_item", "Patch an item while preserving omitted fields.", func(ctx context.Context, in updateInput) (any, error) { return updateItem(ctx, svc, in) })
	add(s, "move_item", "Change an item's workflow status while preserving its other fields.", func(ctx context.Context, in moveInput) (any, error) {
		if !validStatus(in.Status) {
			return nil, service.ErrInvalid
		}
		return updateItem(ctx, svc, updateInput{ID: in.ID, Changes: map[string]any{"status": in.Status}})
	})
	add(s, "archive_item", "Soft archive an item by UUID.", func(ctx context.Context, in idInput) (any, error) {
		err := svc.Items().Archive(ctx, "mcp", in.ID)
		return map[string]any{"id": in.ID, "archived": err == nil}, err
	})
	add(s, "upsert_feature", "Create or fully replace a feature; supply its UUID to update it.", func(ctx context.Context, in featureInput) (any, error) {
		if in.Status != "" && in.Status != "implemented" && in.Status != "deprecated" {
			return nil, service.ErrInvalid
		}
		return svc.Features().Put(ctx, "mcp", service.Feature{Record: service.Record{ID: in.ID}, ProjectID: in.ProjectID, Title: in.Title, BodyMd: in.BodyMd, CodeRefs: in.CodeRefs, Status: in.Status})
	})
	add(s, "remember", "Store a memory with global, project or environment scope.", func(ctx context.Context, in memoryInput) (any, error) {
		if !slices.Contains([]string{"global", "project", "environment"}, in.Scope) || in.Content == "" {
			return nil, service.ErrInvalid
		}
		if in.Scope == "global" {
			if in.ScopeID != nil {
				return nil, service.ErrInvalid
			}
		} else if in.ScopeID == nil || *in.ScopeID == "" {
			return nil, service.ErrInvalid
		}
		return svc.Memories().Put(ctx, "mcp", service.Memory{Scope: in.Scope, ScopeID: in.ScopeID, Content: in.Content, Tags: in.Tags})
	})
	add(s, "recall", "Full text search of memories, optionally filtered by scope and scope_id, with pagination.", func(ctx context.Context, in recallInput) (any, error) { return recall(ctx, svc, in) })
	add(s, "link", "Create a directed link between domain entities.", func(ctx context.Context, in linkInput) (any, error) {
		return svc.Links().Put(ctx, "mcp", service.Link{FromID: in.FromID, FromType: in.FromType, ToID: in.ToID, ToType: in.ToType, Kind: in.Kind})
	})
	return sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return s }, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
}
