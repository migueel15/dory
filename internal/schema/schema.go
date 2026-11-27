// Package schema describes the entities shared by all nodes.
package schema

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

type Kind int

const (
	Text Kind = iota
	Int
	Bool
	JSON
	Time // RFC3339Nano text
)

type Column struct {
	Name     string
	Kind     Kind
	Default  any
	Nullable bool
}

type Entity struct {
	Name, Table string
	// Key values form the operation id, joined by "/" for composite keys.
	Key     []string
	Columns []Column
	// Unique collisions are resolved by giving the smallest id the value.
	Unique string
	// Merge preserves concurrent versions of this text column.
	Merge string
}

// ColumnNames returns the mutable columns in replication order.
func (e Entity) ColumnNames() []string {
	names := make([]string, len(e.Columns))
	for i, col := range e.Columns {
		names[i] = col.Name
	}
	return names
}

func (e Entity) Column(name string) (Column, bool) {
	for _, col := range e.Columns {
		if col.Name == name {
			return col, true
		}
	}
	return Column{}, false
}

func (e Entity) IsJSON(name string) bool {
	col, ok := e.Column(name)
	return ok && col.Kind == JSON
}

// Defaults returns the explicit defaults applied on creation.
func (e Entity) Defaults() map[string]any {
	defaults := make(map[string]any)
	for _, col := range e.Columns {
		if col.Default != nil {
			defaults[col.Name] = cloneDefault(col.Default)
		}
	}
	return defaults
}

func cloneDefault(value any) any {
	switch v := value.(type) {
	case map[string]any:
		copy := maps.Clone(v)
		for key, item := range copy {
			copy[key] = cloneDefault(item)
		}
		return copy
	case []any:
		copy := slices.Clone(v)
		for i, item := range copy {
			copy[i] = cloneDefault(item)
		}
		return copy
	default:
		return value
	}
}

func cloneEntity(e Entity) Entity {
	e.Key = slices.Clone(e.Key)
	e.Columns = slices.Clone(e.Columns)
	for i, col := range e.Columns {
		e.Columns[i].Default = cloneDefault(col.Default)
	}
	return e
}

func Lookup(name string) (Entity, bool) {
	for _, e := range registry {
		if e.Name == name {
			return cloneEntity(e), true
		}
	}
	return Entity{}, false
}

func All() []Entity {
	entities := make([]Entity, len(registry))
	for i, e := range registry {
		entities[i] = cloneEntity(e)
	}
	return entities
}

// Hash identifies the ordered schema, including defaults and conflict rules.
func Hash() string {
	raw, err := json.Marshal(registry)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

var registry = []Entity{
	{Name: "project", Table: "projects", Key: []string{"id"},
		Unique: "slug",
		Columns: []Column{
			{Name: "slug", Kind: Text},
			{Name: "name", Kind: Text},
			{Name: "description", Kind: Text},
			{Name: "created_at", Kind: Time},
			{Name: "updated_at", Kind: Time},
			{Name: "archived_at", Kind: Time, Nullable: true},
		},
	},
	{Name: "repository", Table: "repositories", Key: []string{"id"},
		Columns: []Column{
			{Name: "project_id", Kind: Text},
			{Name: "remote_url", Kind: Text},
			{Name: "default_branch", Kind: Text, Default: "main"},
			{Name: "created_at", Kind: Time},
			{Name: "updated_at", Kind: Time},
			{Name: "archived_at", Kind: Time, Nullable: true},
		},
	},
	{Name: "repository_location", Table: "repository_locations", Key: []string{"repository_id", "node_id"},
		Columns: []Column{
			{Name: "path", Kind: Text},
			{Name: "created_at", Kind: Time},
			{Name: "updated_at", Kind: Time},
			{Name: "archived_at", Kind: Time, Nullable: true},
		},
	},
	{Name: "task", Table: "tasks", Key: []string{"id"},
		Columns: []Column{
			{Name: "source", Kind: Text, Default: "internal"},
			{Name: "external_id", Kind: Text},
			{Name: "project_id", Kind: Text, Nullable: true},
			{Name: "repository_id", Kind: Text, Nullable: true},
			{Name: "number", Kind: Int, Nullable: true},
			{Name: "title", Kind: Text},
			{Name: "description", Kind: Text},
			{Name: "stage", Kind: Text, Default: "inbox"},
			{Name: "branch", Kind: Text},
			{Name: "issue_number", Kind: Int, Nullable: true},
			{Name: "pr_number", Kind: Int, Nullable: true},
			{Name: "position", Kind: Text},
			{Name: "created_at", Kind: Time},
			{Name: "updated_at", Kind: Time},
			{Name: "archived_at", Kind: Time, Nullable: true},
		},
	},
	{Name: "thread_pull", Table: "thread_pulls", Key: []string{"thread_id", "repository_id", "number"},
		Columns: []Column{
			{Name: "branch", Kind: Text},
			{Name: "linked_by", Kind: Text, Default: "manual"},
			{Name: "created_at", Kind: Time},
			{Name: "updated_at", Kind: Time},
			{Name: "archived_at", Kind: Time, Nullable: true},
		},
	},
	{Name: "task_dependency", Table: "task_dependencies", Key: []string{"task_id", "depends_on_id"},
		Columns: []Column{
			{Name: "created_at", Kind: Time},
			{Name: "archived_at", Kind: Time, Nullable: true},
		},
	},
	{Name: "plan", Table: "plans", Key: []string{"id"},
		Columns: []Column{
			{Name: "task_id", Kind: Text},
			{Name: "version", Kind: Int},
			{Name: "sections", Kind: JSON, Default: map[string]any{}},
			{Name: "open_questions", Kind: JSON, Default: []any{}},
			{Name: "author", Kind: Text, Default: "agent"},
			{Name: "accepted_at", Kind: Time, Nullable: true},
			{Name: "body_md", Kind: Text},
			{Name: "revision", Kind: Int},
			{Name: "created_at", Kind: Time},
			{Name: "updated_at", Kind: Time},
			{Name: "archived_at", Kind: Time, Nullable: true},
		},
	},
	{Name: "memory", Table: "memories", Key: []string{"id"},
		Merge: "content",
		Columns: []Column{
			{Name: "scope", Kind: Text},
			{Name: "scope_id", Kind: Text},
			{Name: "kind", Kind: Text, Default: "context"},
			{Name: "title", Kind: Text},
			{Name: "content", Kind: Text},
			{Name: "labels", Kind: JSON, Default: []any{}},
			{Name: "created_at", Kind: Time},
			{Name: "updated_at", Kind: Time},
			{Name: "archived_at", Kind: Time, Nullable: true},
		},
	},
	{Name: "skill", Table: "skills", Key: []string{"id"},
		Unique: "name",
		Columns: []Column{
			{Name: "name", Kind: Text},
			{Name: "description", Kind: Text},
			{Name: "body_md", Kind: Text},
			{Name: "files", Kind: JSON, Default: map[string]any{}},
			{Name: "version", Kind: Int},
			{Name: "targets", Kind: JSON, Default: map[string]any{}},
			{Name: "source", Kind: Text},
			{Name: "system_version", Kind: Int},
			{Name: "forked_from", Kind: Text, Nullable: true},
			{Name: "disabled_at", Kind: Time, Nullable: true},
			{Name: "created_at", Kind: Time},
			{Name: "updated_at", Kind: Time},
			{Name: "archived_at", Kind: Time, Nullable: true},
		},
	},
	{Name: "skill_state", Table: "skill_states", Key: []string{"skill_id", "node_id"},
		Columns: []Column{
			{Name: "status", Kind: Text},
			{Name: "version", Kind: Int},
			{Name: "message", Kind: Text},
			{Name: "created_at", Kind: Time},
			{Name: "updated_at", Kind: Time},
			{Name: "archived_at", Kind: Time, Nullable: true},
		},
	},
	{Name: "notice", Table: "notices", Key: []string{"id"},
		Columns: []Column{
			{Name: "kind", Kind: Text},
			{Name: "title", Kind: Text},
			{Name: "body", Kind: Text},
			{Name: "url", Kind: Text},
			{Name: "read_at", Kind: Time, Nullable: true},
			{Name: "created_at", Kind: Time},
			{Name: "updated_at", Kind: Time},
			{Name: "archived_at", Kind: Time, Nullable: true},
		},
	},
	{Name: "setting", Table: "settings", Key: []string{"id"},
		Columns: []Column{
			{Name: "value", Kind: Text},
			{Name: "created_at", Kind: Time},
			{Name: "updated_at", Kind: Time},
			{Name: "archived_at", Kind: Time, Nullable: true},
		},
	},
	{Name: "thread", Table: "threads", Key: []string{"id"},
		Columns: []Column{
			{Name: "node_id", Kind: Text},
			{Name: "project_id", Kind: Text, Nullable: true},
			{Name: "task_id", Kind: Text, Nullable: true},
			{Name: "task_linked_at", Kind: Time, Nullable: true},
			{Name: "parent_id", Kind: Text, Nullable: true},
			{Name: "origin_id", Kind: Text, Nullable: true},
			{Name: "provider", Kind: Text},
			{Name: "model", Kind: Text},
			{Name: "effort", Kind: Text},
			{Name: "runtime_mode", Kind: Text, Default: "full-access"},
			{Name: "title", Kind: Text},
			{Name: "branch", Kind: Text},
			{Name: "pinned_at", Kind: Time, Nullable: true},
			{Name: "read_at", Kind: Time, Nullable: true},
			{Name: "snoozed_until", Kind: Time, Nullable: true},
			{Name: "settled_at", Kind: Time, Nullable: true},
			{Name: "created_at", Kind: Time},
			{Name: "updated_at", Kind: Time},
			{Name: "archived_at", Kind: Time, Nullable: true},
		},
	},
}
