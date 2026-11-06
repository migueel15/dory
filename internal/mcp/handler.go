// Package mcp exposes dory tools over streamable HTTP. The caller must wrap
// Handler with bearer authentication before mounting it at /mcp.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/migueel15/dory/internal/auth"
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

type itemListInput struct {
	viewListInput
	Open bool `json:"open,omitempty" jsonschema:"Only items not done or cancelled"`
}

// itemFilter moves status and id arrays out of the containment filter into
// AnyOf, and narrows status to open states when open is set.
func (in itemListInput) itemFilter() (service.Filter, error) {
	f := in.filter()
	f.Fields = map[string]any{}
	f.AnyOf = map[string][]string{}
	for key, value := range in.Fields {
		values, isList := value.([]any)
		if !isList || key == "tags" || key == "refs" {
			f.Fields[key] = value
			continue
		}
		if key != "status" && key != "id" {
			return f, fmt.Errorf("%w: only status and id accept several values", service.ErrInvalid)
		}
		for _, v := range values {
			text, ok := v.(string)
			if !ok {
				return f, fmt.Errorf("%w: %s values must be strings", service.ErrInvalid, key)
			}
			f.AnyOf[key] = append(f.AnyOf[key], text)
		}
	}
	if in.Open {
		statuses := f.AnyOf["status"]
		if status, ok := f.Fields["status"].(string); ok {
			statuses = []string{status}
			delete(f.Fields, "status")
		} else if statuses == nil {
			statuses = []string{"inbox", "triaged", "ready", "in_progress", "validating", "blocked"}
		}
		f.AnyOf["status"] = slices.DeleteFunc(slices.Clone(statuses), func(s string) bool { return s == "done" || s == "cancelled" })
	}
	return f, nil
}

func (in listInput) filter() service.Filter {
	return service.Filter{Fields: in.Fields, Limit: in.Limit, Offset: in.Offset, IncludeArchived: in.IncludeArchived}
}

// add uses the SDK's inferred input schema and reports service failures as tool
// errors. Results include JSON text for clients that do not read structured data.
func add[I any](server *sdk.Server, name, description string, fn func(context.Context, I) (any, error)) {
	sdk.AddTool(server, &sdk.Tool{Name: name, Description: description}, func(ctx context.Context, _ *sdk.CallToolRequest, in I) (*sdk.CallToolResult, any, error) {
		scope := "write"
		if strings.HasPrefix(name, "list_") || strings.HasPrefix(name, "get_") || name == "recall" || name == "session_start" {
			scope = "read"
		}
		if !auth.Allowed(ctx, scope) {
			return nil, nil, fmt.Errorf("permiso %q requerido", scope)
		}
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
func viewListTool[T, S any](s *sdk.Server, name, description string, r service.Resource[T], summarize func(T) S) {
	add(s, name, description, func(ctx context.Context, in viewListInput) (any, error) {
		summary, err := checkView(in.View)
		if err != nil {
			return nil, err
		}
		rows, err := r.List(ctx, in.filter())
		if err != nil || !summary {
			return rows, err
		}
		return mapAll(rows, summarize), nil
	})
}
func getTool[T any](s *sdk.Server, name, description string, r service.Resource[T]) {
	add(s, name, description, func(ctx context.Context, in idInput) (any, error) { return r.Get(ctx, in.ID) })
}

type projectInput struct {
	Project   string `json:"project,omitempty" jsonschema:"Optional project UUID or slug"`
	RemoteURL string `json:"remote_url,omitempty" jsonschema:"Optional git remote URL used to find the project when project is omitted"`
	Path      string `json:"path,omitempty" jsonschema:"Optional local checkout path used to find the project when project and remote_url are omitted"`
}

// normalizeRemote reduces https, ssh and scp-like git URLs to host/owner/repo.
func normalizeRemote(url string) string {
	url = strings.ToLower(strings.TrimSpace(url))
	if i := strings.Index(url, "://"); i >= 0 {
		url = url[i+3:]
	} else if at := strings.Index(url, "@"); at >= 0 {
		url = strings.Replace(url[at+1:], ":", "/", 1)
	}
	if at := strings.Index(url, "@"); at >= 0 && at < strings.Index(url+"/", "/") {
		url = url[at+1:]
	}
	url = strings.TrimSuffix(strings.TrimSuffix(url, "/"), ".git")
	if host, rest, ok := strings.Cut(url, "/"); ok {
		host, _, _ = strings.Cut(host, ":")
		url = host + "/" + rest
	}
	return url
}

// findProject resolves a project id from a remote URL, or from the repository
// location whose path is the longest prefix of path. It returns "" when nothing matches.
func findProject(ctx context.Context, svc service.Service, remoteURL, path string) (string, error) {
	repos, err := all(ctx, svc.Repositories(), service.Filter{})
	if err != nil {
		return "", err
	}
	if remoteURL != "" {
		want := normalizeRemote(remoteURL)
		for _, repo := range repos {
			if normalizeRemote(repo.RemoteUrl) == want {
				return repo.ProjectID, nil
			}
		}
		return "", nil
	}
	locations, err := all(ctx, svc.RepositoryLocations(), service.Filter{})
	if err != nil {
		return "", err
	}
	path = strings.TrimSuffix(path, "/")
	best, repoID := -1, ""
	for _, loc := range locations {
		root := strings.TrimSuffix(loc.Path, "/")
		if (path == root || strings.HasPrefix(path, root+"/")) && len(root) > best {
			best, repoID = len(root), loc.RepositoryID
		}
	}
	for _, repo := range repos {
		if repo.ID == repoID {
			return repo.ProjectID, nil
		}
	}
	return "", nil
}

// featureSummary adds the count of open items so callers know when to close a feature.
type featureSummary struct {
	service.Feature
	OpenItems int `json:"open_items"`
}
type projectDetail struct {
	Project      service.Project      `json:"project"`
	Repositories []service.Repository `json:"repositories"`
	Features     []featureSummary     `json:"features"`
	Memories     []memorySummary      `json:"memories"`
	OpenItems    []itemSummary        `json:"open_items"`
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
	features, err := all(ctx, svc.Features(), f)
	if err != nil {
		return result, err
	}
	memories, err := all(ctx, svc.Memories(), service.Filter{Fields: map[string]any{"scope": "project", "scope_id": result.Project.ID}, Limit: 1000})
	if err != nil {
		return result, err
	}
	result.Memories = mapAll(memories, summarizeMemory)
	items, err := all(ctx, svc.Items(), f)
	if err != nil {
		return result, err
	}
	open := map[string]int{}
	result.OpenItems = []itemSummary{}
	for _, item := range items {
		if item.Status != "done" && item.Status != "cancelled" {
			result.OpenItems = append(result.OpenItems, summarizeItem(item))
			if item.FeatureID != nil {
				open[*item.FeatureID]++
			}
		}
	}
	result.Features = []featureSummary{}
	for _, feature := range features {
		result.Features = append(result.Features, featureSummary{Feature: feature, OpenItems: open[feature.ID]})
	}
	return result, nil
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
	ID            string   `json:"id,omitempty" jsonschema:"Optional client UUID for idempotent upsert"`
	ProjectID     *string  `json:"project_id,omitempty"`
	FeatureID     *string  `json:"feature_id,omitempty" jsonschema:"Optional feature the item belongs to"`
	RawText       string   `json:"raw_text,omitempty"`
	Title         string   `json:"title,omitempty" jsonschema:"Optional; derived from the first line of raw_text (max 80 chars) when missing"`
	BodyMd        string   `json:"body_md,omitempty"`
	Progress      string   `json:"progress,omitempty" jsonschema:"Short current state (branch, last commit, what is next); replaced, not appended"`
	Refs          []string `json:"refs,omitempty" jsonschema:"External references: branch, PR or issue URLs, commits"`
	Tags          []string `json:"tags,omitempty"`
	Status        string   `json:"status,omitempty" jsonschema:"inbox, triaged, ready, in_progress, validating, blocked, done or cancelled"`
	BlockedReason string   `json:"blocked_reason,omitempty" jsonschema:"Required when status is blocked: what blocks it and what unblocks it"`
	Priority      int32    `json:"priority,omitempty"`
	Source        string   `json:"source,omitempty"`
	TitleSource   string   `json:"title_source,omitempty" jsonschema:"pending, ai or user; only \"ai\" is meant for agents, and only on an item whose title is pending"`
}
type updateInput struct {
	ID      string         `json:"id"`
	Changes map[string]any `json:"changes" jsonschema:"Patch fields: project_id and feature_id (nullable), raw_text, title, body_md, progress (short current state, replaced), refs, tags, status, blocked_reason (required for blocked, cleared otherwise), priority, source, title_source (\"ai\" only when the title is pending). Omitted fields are preserved."`
}
type updatesInput struct {
	Updates []updateInput  `json:"updates,omitempty" jsonschema:"Per-item patches: [{id, changes}]"`
	IDs     []string       `json:"ids,omitempty" jsonschema:"Items that all receive the same changes"`
	Changes map[string]any `json:"changes,omitempty" jsonschema:"Patch applied to every id in ids; same fields as update_item"`
}
type moveInput struct {
	ID            string `json:"id"`
	Status        string `json:"status" jsonschema:"inbox, triaged, ready, in_progress, validating, blocked, done or cancelled"`
	BlockedReason string `json:"blocked_reason,omitempty" jsonschema:"Required when status is blocked"`
}

func validStatus(status string) bool {
	return slices.Contains([]string{"inbox", "triaged", "ready", "in_progress", "validating", "blocked", "done", "cancelled"}, status)
}
func validFeatureStatus(status string) bool {
	return slices.Contains([]string{"planned", "in_progress", "implemented", "deprecated"}, status)
}

// deriveTitle takes the first non-empty line of text, cut to 80 characters.
func deriveTitle(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			if r := []rune(line); len(r) > 80 {
				return strings.TrimSpace(string(r[:80]))
			}
			return line
		}
	}
	return ""
}
func putItem(ctx context.Context, svc service.Service, item service.Item) (service.Item, error) {
	if item.Status != "" && !validStatus(item.Status) {
		return service.Item{}, fmt.Errorf("%w: unknown item status", service.ErrInvalid)
	}
	// A blocked item must say why; leaving the state clears the reason.
	if item.Status == "blocked" && strings.TrimSpace(item.BlockedReason) == "" {
		return service.Item{}, fmt.Errorf("%w: blocked_reason is required for blocked items", service.ErrInvalid)
	}
	if item.Status != "blocked" {
		item.BlockedReason = ""
	}
	return svc.Items().Put(ctx, auth.Actor(ctx), item)
}

// updateItems applies every patch in one transaction: one failure undoes all.
func updateItems(ctx context.Context, svc service.Service, in updatesInput) (any, error) {
	updates := in.Updates
	for _, id := range in.IDs {
		updates = append(updates, updateInput{ID: id, Changes: in.Changes})
	}
	if len(updates) == 0 || len(updates) > 1000 {
		return nil, fmt.Errorf("%w: send between 1 and 1000 updates", service.ErrInvalid)
	}
	result := make([]any, 0, len(updates))
	err := svc.InTx(ctx, func(tx service.Service) error {
		for i, u := range updates {
			item, err := updateItem(ctx, tx, u)
			if err != nil {
				return fmt.Errorf("updates[%d] %s: %w", i, u.ID, err)
			}
			short, _ := writeReturn{}.reply(item, nil)
			result = append(result, short)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
func updateItem(ctx context.Context, svc service.Service, in updateInput) (service.Item, error) {
	item, err := svc.Items().Get(ctx, in.ID)
	if err != nil {
		return service.Item{}, err
	}
	raw, err := json.Marshal(item)
	if err != nil {
		return service.Item{}, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil {
		return service.Item{}, err
	}
	for key, value := range in.Changes {
		if !slices.Contains([]string{"project_id", "feature_id", "raw_text", "title", "body_md", "progress", "refs", "tags", "status", "blocked_reason", "priority", "source", "title_source"}, key) {
			return service.Item{}, fmt.Errorf("%w: field %s cannot be changed", service.ErrInvalid, key)
		}
		if value == nil && key != "project_id" && key != "feature_id" {
			return service.Item{}, fmt.Errorf("%w: only project_id and feature_id may be null", service.ErrInvalid)
		}
		fields[key], err = json.Marshal(value)
		if err != nil {
			return service.Item{}, err
		}
	}
	raw, err = json.Marshal(fields)
	if err != nil {
		return service.Item{}, err
	}
	if err = json.Unmarshal(raw, &item); err != nil {
		return service.Item{}, fmt.Errorf("%w: %v", service.ErrInvalid, err)
	}
	return putItem(ctx, svc, item)
}

type featureInput struct {
	ID        string   `json:"id,omitempty"`
	ProjectID string   `json:"project_id"`
	Title     string   `json:"title"`
	BodyMd    string   `json:"body_md,omitempty"`
	CodeRefs  []string `json:"code_refs,omitempty"`
	Status    string   `json:"status,omitempty" jsonschema:"planned (default), in_progress, implemented or deprecated"`
}
type featureUpdate struct {
	ID       string    `json:"id" jsonschema:"Feature UUID"`
	Title    *string   `json:"title,omitempty"`
	BodyMd   *string   `json:"body_md,omitempty"`
	CodeRefs *[]string `json:"code_refs,omitempty"`
	Status   *string   `json:"status,omitempty" jsonschema:"planned, in_progress, implemented or deprecated"`
}
type memoryInput struct {
	ID      string   `json:"id,omitempty" jsonschema:"Optional client UUID for idempotent upsert"`
	Scope   string   `json:"scope" jsonschema:"global, project or environment"`
	ScopeID *string  `json:"scope_id,omitempty" jsonschema:"Required UUID for project or environment scope"`
	Kind    string   `json:"kind,omitempty" jsonschema:"decision, lesson or context (default)"`
	Title   string   `json:"title,omitempty" jsonschema:"Short title for listings"`
	Content string   `json:"content"`
	Tags    []string `json:"tags,omitempty"`
}
type recallInput struct {
	Query   string   `json:"query,omitempty" jsonschema:"Full text query; optional when tags are given"`
	Tags    []string `json:"tags,omitempty" jsonschema:"Only memories that carry all these tags"`
	Kind    string   `json:"kind,omitempty" jsonschema:"Optional decision, lesson or context"`
	Scope   string   `json:"scope,omitempty" jsonschema:"Optional global, project or environment scope"`
	ScopeID string   `json:"scope_id,omitempty"`
	Limit   int32    `json:"limit,omitempty" jsonschema:"Default 100 maximum 1000"`
	Offset  int32    `json:"offset,omitempty"`
	View    string   `json:"view,omitempty" jsonschema:"summary (default: id, kind, title, tags, first 200 characters of content) or full"`
}

// recall applies the requested view to the matching memories.
func recall(ctx context.Context, svc service.Service, in recallInput) (any, error) {
	summary, err := checkView(in.View)
	if err != nil {
		return nil, err
	}
	hits, err := recallHits(ctx, svc, in)
	if err != nil || !summary {
		return hits, err
	}
	return summarizeHits(hits)
}
func recallHits(ctx context.Context, svc service.Service, in recallInput) ([]service.SearchHit, error) {
	if (in.Query == "" && len(in.Tags) == 0 && in.Kind == "") || !validMemoryKind(in.Kind) || in.Offset < 0 || in.Limit < 0 || in.Limit > 1000 {
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
	if in.Query == "" {
		return recallByTags(ctx, svc, in, limit)
	}
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
			if !hasTags(memory.Tags, in.Tags) || (in.Kind != "" && memory.Kind != in.Kind) {
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

// recallByTags lists memories by tags and kind when there is no text query.
func recallByTags(ctx context.Context, svc service.Service, in recallInput, limit int32) ([]service.SearchHit, error) {
	fields := map[string]any{}
	if len(in.Tags) > 0 {
		fields["tags"] = in.Tags
	}
	if in.Kind != "" {
		fields["kind"] = in.Kind
	}
	if in.Scope != "" {
		fields["scope"] = in.Scope
	}
	if in.ScopeID != "" {
		fields["scope_id"] = in.ScopeID
	}
	memories, err := svc.Memories().List(ctx, service.Filter{Fields: fields, Limit: limit, Offset: in.Offset})
	if err != nil {
		return nil, err
	}
	result := []service.SearchHit{}
	for _, memory := range memories {
		raw, err := json.Marshal(memory)
		if err != nil {
			return nil, err
		}
		result = append(result, service.SearchHit{EntityType: "memory", EntityID: memory.ID, Content: raw})
	}
	return result, nil
}
func hasTags(have, want []string) bool {
	for _, tag := range want {
		if !slices.Contains(have, tag) {
			return false
		}
	}
	return true
}

func createItem(ctx context.Context, svc service.Service, in itemInput) (service.Item, error) {
	if in.Source == "" {
		in.Source = "mcp"
	}
	if strings.TrimSpace(in.Title) == "" {
		in.Title = deriveTitle(in.RawText)
	}
	return putItem(ctx, svc, service.Item{Record: service.Record{ID: in.ID}, ProjectID: in.ProjectID, FeatureID: in.FeatureID, RawText: in.RawText, Title: in.Title, BodyMd: in.BodyMd, Progress: in.Progress, Refs: in.Refs, Tags: in.Tags, Status: in.Status, BlockedReason: in.BlockedReason, Priority: in.Priority, Source: in.Source, TitleSource: in.TitleSource})
}
func putFeature(ctx context.Context, svc service.Service, in featureInput) (service.Feature, error) {
	if in.Status != "" && !validFeatureStatus(in.Status) {
		return service.Feature{}, service.ErrInvalid
	}
	return svc.Features().Put(ctx, auth.Actor(ctx), service.Feature{Record: service.Record{ID: in.ID}, ProjectID: in.ProjectID, Title: in.Title, BodyMd: in.BodyMd, CodeRefs: in.CodeRefs, Status: in.Status})
}
func putMemory(ctx context.Context, svc service.Service, in memoryInput) (service.Memory, error) {
	if in.Content == "" || !validMemoryKind(in.Kind) || !validMemoryScope(in.Scope, in.ScopeID) {
		return service.Memory{}, service.ErrInvalid
	}
	return svc.Memories().Put(ctx, auth.Actor(ctx), service.Memory{Record: service.Record{ID: in.ID}, Scope: in.Scope, ScopeID: in.ScopeID, Kind: in.Kind, Title: in.Title, Content: in.Content, Tags: in.Tags})
}

type batchInput struct {
	Features []featureInput `json:"features,omitempty"`
	Items    []itemInput    `json:"items,omitempty"`
	Memories []memoryInput  `json:"memories,omitempty"`
	Links    []linkInput    `json:"links,omitempty"`
}

// importBatch writes every record inside one transaction and reports the ids
// in input order; the error names the failing record.
func importBatch(ctx context.Context, svc service.Service, in batchInput) (any, error) {
	if total := len(in.Features) + len(in.Items) + len(in.Memories) + len(in.Links); total == 0 || total > 1000 {
		return nil, fmt.Errorf("%w: a batch needs between 1 and 1000 records", service.ErrInvalid)
	}
	result := map[string][]string{"features": {}, "items": {}, "memories": {}, "links": {}}
	err := svc.InTx(ctx, func(tx service.Service) error {
		for i, f := range in.Features {
			v, err := putFeature(ctx, tx, f)
			if err != nil {
				return fmt.Errorf("features[%d]: %w", i, err)
			}
			result["features"] = append(result["features"], v.ID)
		}
		for i, item := range in.Items {
			v, err := createItem(ctx, tx, item)
			if err != nil {
				return fmt.Errorf("items[%d]: %w", i, err)
			}
			result["items"] = append(result["items"], v.ID)
		}
		for i, m := range in.Memories {
			v, err := putMemory(ctx, tx, m)
			if err != nil {
				return fmt.Errorf("memories[%d]: %w", i, err)
			}
			result["memories"] = append(result["memories"], v.ID)
		}
		for i, l := range in.Links {
			v, err := tx.Links().Put(ctx, auth.Actor(ctx), service.Link{FromID: l.FromID, FromType: l.FromType, ToID: l.ToID, ToType: l.ToType, Kind: l.Kind})
			if err != nil {
				return fmt.Errorf("links[%d]: %w", i, err)
			}
			result["links"] = append(result["links"], v.ID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

type linkInput struct {
	FromID   string `json:"from_id"`
	FromType string `json:"from_type"`
	ToID     string `json:"to_id"`
	ToType   string `json:"to_type"`
	Kind     string `json:"kind"`
}

type projectUpsert struct {
	ID          string    `json:"id,omitempty" jsonschema:"Existing project UUID to update; omit to create"`
	Slug        *string   `json:"slug,omitempty"`
	Name        *string   `json:"name,omitempty"`
	Description *string   `json:"description,omitempty"`
	Status      *string   `json:"status,omitempty" jsonschema:"Project status, e.g. active"`
	Stack       *[]string `json:"stack,omitempty"`
}
type memoryUpdate struct {
	ID      string    `json:"id" jsonschema:"Memory UUID"`
	Kind    *string   `json:"kind,omitempty" jsonschema:"decision, lesson or context"`
	Title   *string   `json:"title,omitempty"`
	Content *string   `json:"content,omitempty"`
	Tags    *[]string `json:"tags,omitempty"`
	Scope   *string   `json:"scope,omitempty" jsonschema:"global, project or environment"`
	ScopeID *string   `json:"scope_id,omitempty" jsonschema:"UUID required for project or environment scope"`
}
type repositoryUpsert struct {
	ID            string  `json:"id,omitempty" jsonschema:"Existing repository UUID to update; omit to create"`
	ProjectID     *string `json:"project_id,omitempty"`
	RemoteURL     *string `json:"remote_url,omitempty"`
	DefaultBranch *string `json:"default_branch,omitempty"`
}
type threadLinkInput struct {
	ItemID     string `json:"item_id"`
	ThreadID   string `json:"thread_id"`
	InstanceID string `json:"instance_id,omitempty" jsonschema:"T3 instance UUID; found from the thread when omitted"`
	Role       string `json:"role,omitempty" jsonschema:"implementation (default), review or refine"`
}

func linkThread(ctx context.Context, svc service.Service, locate ThreadLocator, in threadLinkInput) (any, error) {
	if in.ItemID == "" || in.ThreadID == "" {
		return nil, service.ErrInvalid
	}
	if in.InstanceID == "" {
		id, ok := locate(in.ThreadID)
		if !ok {
			return nil, fmt.Errorf("%w: thread not found in any reachable instance; pass instance_id", service.ErrInvalid)
		}
		in.InstanceID = id
	}
	existing, err := svc.ItemThreads().List(ctx, service.Filter{Fields: map[string]any{"item_id": in.ItemID, "instance_id": in.InstanceID, "thread_id": in.ThreadID}, Limit: 1})
	if err != nil {
		return nil, err
	}
	link := service.ItemThread{ItemID: in.ItemID, InstanceID: in.InstanceID, ThreadID: in.ThreadID, Role: in.Role}
	if len(existing) > 0 {
		link.ID = existing[0].ID
		if in.Role == "" {
			link.Role = existing[0].Role
		}
	}
	return svc.ItemThreads().Put(ctx, auth.Actor(ctx), link)
}

type locationInput struct {
	RepositoryID  string `json:"repository_id"`
	EnvironmentID string `json:"environment_id"`
	Path          string `json:"path" jsonschema:"Absolute checkout path in that environment"`
}

// set overwrites dst only when the caller sent the field.
func set[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}

// loadOrNew returns the existing record when id is given, or a zero value to create.
func loadOrNew[T any](ctx context.Context, r service.Resource[T], id string) (T, error) {
	var zero T
	if id == "" {
		return zero, nil
	}
	return r.Get(ctx, id)
}
func archiveTool[T any](s *sdk.Server, name, description string, r service.Resource[T]) {
	add(s, name, description, func(ctx context.Context, in idInput) (any, error) {
		err := r.Archive(ctx, auth.Actor(ctx), in.ID)
		return map[string]any{"id": in.ID, "archived": err == nil}, err
	})
}
func validMemoryKind(kind string) bool {
	return kind == "" || slices.Contains([]string{"decision", "lesson", "context"}, kind)
}
func validMemoryScope(scope string, scopeID *string) bool {
	if !slices.Contains([]string{"global", "project", "environment"}, scope) {
		return false
	}
	if scope == "global" {
		return scopeID == nil
	}
	return scopeID != nil && *scopeID != ""
}

// Handler returns the transport mounted by cmd/dory at /mcp. Stateless requests
// avoid keeping authentication-independent sessions between HTTP requests.
// ThreadLocator finds the T3 instance that reports a thread.
type ThreadLocator func(threadID string) (string, bool)

// itemWithPlan is an item and its newest plan version.
type itemWithPlan struct {
	service.Item
	Plan *service.ItemPlan `json:"plan"`
}

type planInput struct {
	ItemID        string               `json:"item_id" jsonschema:"Item UUID"`
	Sections      service.PlanSections `json:"sections" jsonschema:"Plan sections in markdown; at least one is required"`
	OpenQuestions []string             `json:"open_questions,omitempty" jsonschema:"Questions the user still has to answer; the plan cannot be accepted while any remain"`
}

func Handler(svc service.Service, locate ThreadLocator) http.Handler {
	s := sdk.NewServer(&sdk.Implementation{Name: "dory", Version: "0.1.0"}, nil)
	viewListTool(s, "list_projects", "List projects with optional field filters and pagination; view summary (default) or full.", svc.Projects(), summarizeProject)
	add(s, "list_items", "List items with optional field filters and pagination; view summary (default: id, title, status, project_id, feature_id, priority, tags, updated_at) or full. fields.status and fields.id accept an array meaning any of; open true excludes done and cancelled. get_item returns the whole item.", func(ctx context.Context, in itemListInput) (any, error) {
		summary, err := checkView(in.View)
		if err != nil {
			return nil, err
		}
		f, err := in.itemFilter()
		if err != nil {
			return nil, err
		}
		rows, err := svc.Items().List(ctx, f)
		if err != nil || !summary {
			return rows, err
		}
		return mapAll(rows, summarizeItem), nil
	})
	add(s, "get_item", "Get an unarchived item by UUID, with plan holding its newest plan version (null if none).", func(ctx context.Context, in idInput) (any, error) {
		item, err := svc.Items().Get(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		plan, err := service.LatestPlan(ctx, svc, in.ID)
		return itemWithPlan{item, plan}, err
	})
	add(s, "propose_plan", "Store a new version of an item plan with its fixed sections (goal, scope, where, design, requirements, validation, risks; markdown) and the open_questions still pending. It never makes the item ready: an inbox or ready item goes back to triaged, and only the user accepts a plan. Returns {id, item_id, version}.", func(ctx context.Context, in planInput) (any, error) {
		plan, err := service.ProposePlan(ctx, svc, auth.Actor(ctx), in.ItemID, "agent", in.Sections, in.OpenQuestions)
		if err != nil {
			return nil, err
		}
		return map[string]any{"id": plan.ID, "item_id": plan.ItemID, "version": plan.Version}, nil
	})
	viewListTool(s, "list_features", "List features with optional filters, e.g. status planned, in_progress, implemented or deprecated; view summary (default) or full.", svc.Features(), summarizeFeature)
	listTool(s, "list_environments", "List environments with optional filters.", svc.Environments())
	getTool(s, "get_environment", "Get an environment by UUID.", svc.Environments())
	listTool(s, "list_repositories", "List repositories; filter by project_id if needed.", svc.Repositories())
	add(s, "get_project", "Get project by UUID or slug with repositories, features, project memories and open items.", func(ctx context.Context, in struct {
		ID string `json:"id" jsonschema:"Project UUID or slug"`
	}) (any, error) {
		return projectContext(ctx, svc, in.ID)
	})
	add(s, "session_start", "Load global memories and available projects; optionally include project context by UUID or slug, or found from a git remote_url or local path (project_match is none when nothing matches).", func(ctx context.Context, in projectInput) (any, error) {
		memories, err := all(ctx, svc.Memories(), service.Filter{Fields: map[string]any{"scope": "global"}})
		if err != nil {
			return nil, err
		}
		projects, err := all(ctx, svc.Projects(), service.Filter{})
		if err != nil {
			return nil, err
		}
		result := map[string]any{"memories": mapAll(memories, summarizeMemory), "projects": projects}
		if in.Project == "" && (in.RemoteURL != "" || in.Path != "") {
			if in.Project, err = findProject(ctx, svc, in.RemoteURL, in.Path); err != nil {
				return nil, err
			}
			if in.Project == "" {
				result["project_match"] = "none"
			}
		}
		if in.Project != "" {
			detail, err := projectContext(ctx, svc, in.Project)
			if err != nil {
				return nil, err
			}
			result["project"] = detail
		}
		return result, nil
	})
	add(s, "create_item", "Create or idempotently upsert an item; defaults to inbox and source mcp. Returns {id, status, updated_at} unless return is full.", func(ctx context.Context, in struct {
		itemInput
		writeReturn
	}) (any, error) {
		if err := in.check(); err != nil {
			return nil, err
		}
		return in.reply(createItem(ctx, svc, in.itemInput))
	})
	add(s, "update_item", "Patch an item while preserving omitted fields. Returns {id, status, updated_at} unless return is full.", func(ctx context.Context, in struct {
		updateInput
		writeReturn
	}) (any, error) {
		if err := in.check(); err != nil {
			return nil, err
		}
		return in.reply(updateItem(ctx, svc, in.updateInput))
	})
	add(s, "update_items", "Patch up to 1000 items in one transaction, all or nothing: updates [{id, changes}] and/or ids with shared changes. Returns [{id, status, updated_at}].", func(ctx context.Context, in updatesInput) (any, error) {
		return updateItems(ctx, svc, in)
	})
	add(s, "move_item", "Change an item's workflow status while preserving its other fields; blocked needs blocked_reason. Returns {id, status, updated_at} unless return is full.", func(ctx context.Context, in struct {
		moveInput
		writeReturn
	}) (any, error) {
		if err := in.check(); err != nil {
			return nil, err
		}
		if !validStatus(in.Status) {
			return nil, service.ErrInvalid
		}
		changes := map[string]any{"status": in.Status}
		if in.BlockedReason != "" {
			changes["blocked_reason"] = in.BlockedReason
		}
		return in.reply(updateItem(ctx, svc, updateInput{ID: in.ID, Changes: changes}))
	})
	add(s, "archive_item", "Soft archive an item by UUID.", func(ctx context.Context, in idInput) (any, error) {
		err := svc.Items().Archive(ctx, auth.Actor(ctx), in.ID)
		return map[string]any{"id": in.ID, "archived": err == nil}, err
	})
	add(s, "upsert_feature", "Create or fully replace a feature; supply its UUID to update it.", func(ctx context.Context, in featureInput) (any, error) {
		return putFeature(ctx, svc, in)
	})
	add(s, "update_feature", "Edit an existing feature by UUID, changing only the fields sent (title, body_md, code_refs, status).", func(ctx context.Context, in featureUpdate) (any, error) {
		f, err := svc.Features().Get(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		set(&f.Title, in.Title)
		set(&f.BodyMd, in.BodyMd)
		set(&f.CodeRefs, in.CodeRefs)
		set(&f.Status, in.Status)
		if f.Title == "" || !validFeatureStatus(f.Status) {
			return nil, service.ErrInvalid
		}
		return svc.Features().Put(ctx, auth.Actor(ctx), f)
	})
	add(s, "remember", "Store a memory with global, project or environment scope, a kind (decision, lesson or context) and an optional short title.", func(ctx context.Context, in memoryInput) (any, error) {
		return putMemory(ctx, svc, in)
	})
	add(s, "upsert_project", "Create a project, or update one by id changing only the fields sent. Fields: slug, name, description, status, stack.", func(ctx context.Context, in projectUpsert) (any, error) {
		p, err := loadOrNew(ctx, svc.Projects(), in.ID)
		if err != nil {
			return nil, err
		}
		p.ID = in.ID
		set(&p.Slug, in.Slug)
		set(&p.Name, in.Name)
		set(&p.Description, in.Description)
		set(&p.Status, in.Status)
		set(&p.Stack, in.Stack)
		return svc.Projects().Put(ctx, auth.Actor(ctx), p)
	})
	archiveTool(s, "archive_project", "Soft archive a project by UUID.", svc.Projects())
	add(s, "update_memory", "Edit an existing memory by UUID, changing only the fields sent (kind, title, content, tags, scope, scope_id).", func(ctx context.Context, in memoryUpdate) (any, error) {
		m, err := svc.Memories().Get(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		set(&m.Content, in.Content)
		set(&m.Tags, in.Tags)
		set(&m.Kind, in.Kind)
		set(&m.Title, in.Title)
		if in.Scope != nil {
			m.Scope = *in.Scope
			if *in.Scope == "global" && in.ScopeID == nil {
				m.ScopeID = nil
			}
		}
		if in.ScopeID != nil {
			m.ScopeID = in.ScopeID
		}
		if m.Content == "" || !validMemoryKind(m.Kind) || !validMemoryScope(m.Scope, m.ScopeID) {
			return nil, service.ErrInvalid
		}
		return svc.Memories().Put(ctx, auth.Actor(ctx), m)
	})
	archiveTool(s, "archive_memory", "Soft archive a memory by UUID.", svc.Memories())
	add(s, "upsert_repository", "Create a repository, or update one by id changing only the fields sent. Fields: project_id, remote_url, default_branch.", func(ctx context.Context, in repositoryUpsert) (any, error) {
		r, err := loadOrNew(ctx, svc.Repositories(), in.ID)
		if err != nil {
			return nil, err
		}
		r.ID = in.ID
		set(&r.ProjectID, in.ProjectID)
		set(&r.RemoteUrl, in.RemoteURL)
		set(&r.DefaultBranch, in.DefaultBranch)
		return svc.Repositories().Put(ctx, auth.Actor(ctx), r)
	})
	add(s, "set_repository_location", "Set where a repository is checked out in an environment; updates the existing location for that repository and environment or creates it.", func(ctx context.Context, in locationInput) (any, error) {
		if in.RepositoryID == "" || in.EnvironmentID == "" || in.Path == "" {
			return nil, service.ErrInvalid
		}
		existing, err := svc.RepositoryLocations().List(ctx, service.Filter{Fields: map[string]any{"repository_id": in.RepositoryID, "environment_id": in.EnvironmentID}, Limit: 1})
		if err != nil {
			return nil, err
		}
		loc := service.RepositoryLocation{RepositoryID: in.RepositoryID, EnvironmentID: in.EnvironmentID, Path: in.Path}
		if len(existing) > 0 {
			loc.ID = existing[0].ID
		}
		return svc.RepositoryLocations().Put(ctx, auth.Actor(ctx), loc)
	})
	add(s, "link_thread", "Declare the T3 Code thread working on an item. The instance is found from the thread; pass instance_id only if it is not found. Calling it again is harmless. Role: implementation (default), review or refine.", func(ctx context.Context, in threadLinkInput) (any, error) {
		return linkThread(ctx, svc, locate, in)
	})
	add(s, "recall", "Search memories by full text query, tags (all must match) and/or kind, optionally filtered by scope and scope_id, with pagination; view summary (default) or full.", func(ctx context.Context, in recallInput) (any, error) { return recall(ctx, svc, in) })
	add(s, "import_batch", "Atomically create or upsert many features, items, memories and links in one call. Records are written in that order, so items may reference features of the same batch by client UUID. Any error rolls back the whole batch. Maximum 1000 records.", func(ctx context.Context, in batchInput) (any, error) {
		return importBatch(ctx, svc, in)
	})
	add(s, "link", "Create a directed link between domain entities.", func(ctx context.Context, in linkInput) (any, error) {
		return svc.Links().Put(ctx, auth.Actor(ctx), service.Link{FromID: in.FromID, FromType: in.FromType, ToID: in.ToID, ToType: in.ToType, Kind: in.Kind})
	})
	return sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return s }, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
}
