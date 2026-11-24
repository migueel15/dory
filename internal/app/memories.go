package app

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/migueel15/dory/internal/data"
)

// RecallLimit keeps recall answers short: a few relevant memories, not the
// whole store.
const RecallLimit = 8

// Memories saves and finds the agent memories.
type Memories struct {
	Store *data.Store
}

// RememberInput is a memory to save; an ID updates that memory.
type RememberInput struct {
	ID      string
	Scope   string
	ScopeID string
	Kind    string
	Title   string
	Content string
	Labels  []string
}

// RecallInput selects memories; an ID reads that one and ignores the rest.
type RecallInput struct {
	ID        string
	Query     string
	ProjectID string
	Labels    []string
	Kind      string
	Limit     int
}

// MemoryEntry is a memory in an index: it lists what exists and leaves the
// content to Recall.
type MemoryEntry struct {
	ID     string   `json:"id"`
	Kind   string   `json:"kind"`
	Title  string   `json:"title"`
	Labels []string `json:"labels,omitempty"`
}

// Index lists memories without their content.
func (*Memories) Index(memories []data.Memory) []MemoryEntry {
	out := make([]MemoryEntry, 0, len(memories))
	for _, m := range memories {
		out = append(out, MemoryEntry{ID: m.ID, Kind: m.Kind, Title: m.Title, Labels: m.Labels})
	}
	return out
}

// Global are the memories every project shares.
func (a *Memories) Global(ctx context.Context) ([]data.Memory, error) {
	return a.Store.Memories().List(ctx, data.Filter{Fields: map[string]any{"scope": "global"}, Limit: 1000})
}

// OfProject are the memories of one project.
func (a *Memories) OfProject(ctx context.Context, projectID string) ([]data.Memory, error) {
	return a.Store.Memories().List(ctx, data.Filter{Fields: map[string]any{"scope": "project", "scope_id": projectID}, Limit: 1000})
}

// ThreadScope is the project scope of a thread, or global without a thread
// or a project.
func (a *Memories) ThreadScope(ctx context.Context, threadID string) (scope, scopeID string) {
	if threadID == "" {
		return "global", ""
	}
	t, err := a.Store.Threads().Get(ctx, threadID)
	if err != nil || t.ProjectID == nil || *t.ProjectID == "" {
		return "global", ""
	}
	return "project", *t.ProjectID
}

// Remember creates a memory or updates one. The same title in the same scope
// is the same memory.
func (a *Memories) Remember(ctx context.Context, in RememberInput) (data.Memory, error) {
	store := a.Store
	var m data.Memory
	if in.ID != "" {
		var err error
		if m, err = store.Memories().Get(ctx, in.ID); err != nil {
			return m, err
		}
	} else {
		m.Scope = "global"
	}
	if in.Scope != "" {
		m.Scope, m.ScopeID = in.Scope, in.ScopeID
	}
	if in.ID == "" && in.Title != "" {
		same, err := store.Memories().List(ctx, data.Filter{Fields: map[string]any{"scope": m.Scope, "scope_id": m.ScopeID, "title": in.Title}, Limit: 1})
		if err != nil {
			return m, err
		}
		if len(same) > 0 {
			m = same[0]
		}
	}
	fields := data.Fields{}
	if in.Scope != "" {
		fields["scope"], fields["scope_id"] = in.Scope, in.ScopeID
	}
	if in.Kind != "" {
		m.Kind = in.Kind
		fields["kind"] = in.Kind
	}
	if in.Title != "" {
		m.Title = in.Title
		fields["title"] = in.Title
	}
	if in.Content != "" {
		m.Content = in.Content
		fields["content"] = in.Content
	}
	if in.Labels != nil {
		m.Labels = in.Labels
		fields["labels"] = in.Labels
	}
	if m.Content == "" {
		return m, data.ErrInvalid.Msgf("content is required")
	}
	if m.ID == "" {
		return store.Memories().Create(ctx, m)
	}
	return store.Memories().Patch(ctx, m.ID, fields)
}

// Recall finds a few memories by text, project, labels or kind, best match
// first, or reads one by id.
func (a *Memories) Recall(ctx context.Context, in RecallInput) ([]data.Memory, error) {
	store := a.Store
	if in.ID != "" {
		m, err := store.Memories().Get(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		return []data.Memory{m}, nil
	}
	limit := in.Limit
	if limit <= 0 || limit > 100 {
		limit = RecallLimit
	}
	scope := data.MemoryScope{Kind: in.Kind}
	if in.ProjectID != "" {
		scope.Scope, scope.ScopeID = "project", in.ProjectID
	}
	var memories []data.Memory
	if in.Query != "" {
		// Labels are filtered here, so ask for more than the limit.
		hits, err := store.SearchMemories(ctx, in.Query, scope, 200)
		if err != nil {
			return nil, err
		}
		for _, h := range hits {
			var m data.Memory
			if json.Unmarshal(h.Record, &m) == nil {
				memories = append(memories, m)
			}
		}
	} else {
		fields := map[string]any{}
		for k, v := range map[string]string{"scope": scope.Scope, "scope_id": scope.ScopeID, "kind": scope.Kind} {
			if v != "" {
				fields[k] = v
			}
		}
		var err error
		if memories, err = store.Memories().List(ctx, data.Filter{Fields: fields, Limit: 1000}); err != nil {
			return nil, err
		}
		slices.Reverse(memories) // newest first
	}
	memories = slices.DeleteFunc(memories, func(m data.Memory) bool {
		for _, l := range in.Labels {
			if !slices.Contains(m.Labels, l) {
				return true
			}
		}
		return false
	})
	if len(memories) > limit {
		memories = memories[:limit]
	}
	return memories, nil
}

// Search looks for tasks and memories by text.
func (a *Memories) Search(ctx context.Context, query string, limit int) ([]data.Hit, error) {
	hits, err := a.Store.Search(ctx, query, limit)
	if hits == nil {
		hits = []data.Hit{}
	}
	return hits, err
}

// SearchTasks looks for tasks by text.
func (a *Memories) SearchTasks(ctx context.Context, query string, limit int) ([]data.Hit, error) {
	hits, err := a.Store.SearchTasks(ctx, query, limit)
	if hits == nil {
		hits = []data.Hit{}
	}
	return hits, err
}
