package localstore

import (
	"fmt"
	"slices"

	"github.com/migueel15/dory/internal/service"
)

type fk struct {
	field, kind string
	required    bool
}
type unique struct {
	fields []string
	live   bool // only among non-archived rows
}

// kindSpec holds what Postgres enforced with defaults, constraints and foreign
// keys; the local store validates it in Go with the same errors.
type kindSpec struct {
	name     string
	defaults map[string]any
	enums    map[string][]string
	fks      []fk
	uuids    []string // other uuid-typed fields; "" becomes null
	uniques  []unique
	check    func(doc map[string]any) error
	order    string
	table    string // records, or auth_records for local-only kinds
}

func str(doc map[string]any, key string) string { s, _ := doc[key].(string); return s }

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{service.ErrInvalid}, args...)...)
}

var specs = map[string]*kindSpec{}

func init() {
	add := func(s *kindSpec) {
		if s.table == "" {
			s.table = "records"
			if s.name == "user" || s.name == "session" || s.name == "token" {
				s.table = "auth_records"
			}
		}
		if s.order == "" {
			s.order = "created_at, id"
		}
		specs[s.name] = s
	}
	add(&kindSpec{name: "project", defaults: map[string]any{"status": "active"},
		uniques: []unique{{fields: []string{"slug"}}}})
	add(&kindSpec{name: "repository", defaults: map[string]any{"default_branch": "main"},
		fks:     []fk{{"project_id", "project", true}},
		uniques: []unique{{fields: []string{"remote_url"}}}})
	add(&kindSpec{name: "environment", uniques: []unique{{fields: []string{"name"}}}})
	add(&kindSpec{name: "repositorylocation",
		fks:     []fk{{"repository_id", "repository", true}, {"environment_id", "environment", true}},
		uniques: []unique{{fields: []string{"repository_id", "environment_id"}}}})
	add(&kindSpec{name: "t3instance", fks: []fk{{"environment_id", "environment", true}},
		uniques: []unique{{fields: []string{"environment_id"}, live: true}}})
	add(&kindSpec{name: "itemthread", defaults: map[string]any{"role": "implementation"},
		enums:   map[string][]string{"role": {"triage", "implementation", "review", "replan", "refine"}},
		fks:     []fk{{"item_id", "item", true}, {"instance_id", "t3instance", true}},
		uniques: []unique{{fields: []string{"item_id", "instance_id", "thread_id"}, live: true}}})
	add(&kindSpec{name: "itemplan", defaults: map[string]any{"author": "agent"},
		enums: map[string][]string{"author": {"agent", "user"}},
		fks:   []fk{{"item_id", "item", true}},
		order: "json_extract(data, '$.item_id'), json_extract(data, '$.version') DESC",
		check: func(doc map[string]any) error {
			if _, ok := doc["sections"].(map[string]any); !ok {
				return invalid("sections must be an object")
			}
			return nil
		}})
	add(&kindSpec{name: "issuethread", defaults: map[string]any{"kind": "define_issue"},
		enums: map[string][]string{"kind": {"define_issue", "resolve_conflicts", "review"}},
		fks:   []fk{{"project_id", "project", false}, {"instance_id", "t3instance", true}},
		order: "created_at DESC, id"})
	add(&kindSpec{name: "feature", defaults: map[string]any{"status": "planned"},
		enums: map[string][]string{"status": {"planned", "in_progress", "implemented", "deprecated"}},
		fks:   []fk{{"project_id", "project", true}}})
	add(&kindSpec{name: "item", defaults: map[string]any{"status": "inbox", "source": "manual", "title_source": "user"},
		enums: map[string][]string{
			"status":       {"inbox", "triaged", "ready", "in_progress", "validating", "blocked", "done", "cancelled"},
			"title_source": {"pending", "ai", "user", "fallback"},
		},
		fks: []fk{{"project_id", "project", false}, {"feature_id", "feature", false}},
		check: func(doc map[string]any) error {
			if (str(doc, "status") == "blocked") != (str(doc, "blocked_reason") != "") {
				return invalid("blocked_reason is required exactly when the item is blocked")
			}
			return nil
		}})
	add(&kindSpec{name: "memory", defaults: map[string]any{"kind": "context"},
		enums: map[string][]string{"scope": {"global", "project", "environment"}, "kind": {"decision", "lesson", "context"}},
		uuids: []string{"scope_id"},
		check: func(doc map[string]any) error {
			if (str(doc, "scope") == "global") != (doc["scope_id"] == nil) {
				return invalid("scope_id must be set exactly when the scope is not global")
			}
			return nil
		}})
	linkTypes := []string{"project", "repository", "environment", "feature", "item", "memory"}
	add(&kindSpec{name: "link", enums: map[string][]string{"from_type": linkTypes, "to_type": linkTypes},
		uuids: []string{"from_id", "to_id"},
		check: func(doc map[string]any) error {
			if doc["from_id"] == nil || doc["to_id"] == nil {
				return invalid("from_id and to_id are required")
			}
			return nil
		},
		uniques: []unique{{fields: []string{"from_id", "from_type", "to_id", "to_type", "kind"}}}})
	add(&kindSpec{name: "user", uniques: []unique{{fields: []string{"username"}}}})
	add(&kindSpec{name: "session", fks: []fk{{"user_id", "user", true}},
		uniques: []unique{{fields: []string{"secret_hash"}}}})
	add(&kindSpec{name: "token", fks: []fk{{"user_id", "user", true}},
		uniques: []unique{{fields: []string{"secret_hash"}}},
		check: func(doc map[string]any) error {
			perms, _ := doc["permissions"].([]any)
			if len(perms) == 0 {
				return invalid("permissions required")
			}
			for _, p := range perms {
				if s, _ := p.(string); !slices.Contains([]string{"read", "write", "admin"}, s) {
					return invalid("unknown permission %v", p)
				}
			}
			return nil
		}})
}
