package data

import "encoding/json"

// Field names match the columns of the v2 schema.

type Project struct {
	ID          string  `json:"id"`
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
	ArchivedAt  *string `json:"archived_at"`
}

// Repository ids derive from the remote: see oplog.RepositoryID.
type Repository struct {
	ID            string  `json:"id"`
	ProjectID     string  `json:"project_id"`
	RemoteURL     string  `json:"remote_url"`
	DefaultBranch string  `json:"default_branch"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
	ArchivedAt    *string `json:"archived_at"`
}

// Task is one piece of work with its branch and pull request. Stage keeps what
// the user decides before work starts: inbox, ready or cancelled.
type Task struct {
	ID           string  `json:"id"`
	ProjectID    *string `json:"project_id"`
	RepositoryID *string `json:"repository_id"`
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	Stage        string  `json:"stage"`
	Branch       string  `json:"branch"`
	IssueNumber  *int64  `json:"issue_number"`
	PRNumber     *int64  `json:"pr_number"`
	Position     string  `json:"position"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
	ArchivedAt   *string `json:"archived_at"`
}

// Dependency says TaskID cannot start until DependsOnID is done.
type Dependency struct {
	TaskID      string  `json:"task_id"`
	DependsOnID string  `json:"depends_on_id"`
	CreatedAt   string  `json:"created_at"`
	ArchivedAt  *string `json:"archived_at"`
}

type Plan struct {
	ID            string          `json:"id"`
	TaskID        string          `json:"task_id"`
	Version       int64           `json:"version"`
	Sections      json.RawMessage `json:"sections"`
	OpenQuestions []string        `json:"open_questions"`
	// BodyMD is the plan document; Revision counts its saves.
	BodyMD     string  `json:"body_md"`
	Revision   int64   `json:"revision"`
	Author     string  `json:"author"`
	AcceptedAt *string `json:"accepted_at"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
	ArchivedAt *string `json:"archived_at"`
}

// Memory scope is global, project or node; ScopeID is empty for global.
type Memory struct {
	ID         string   `json:"id"`
	Scope      string   `json:"scope"`
	ScopeID    string   `json:"scope_id"`
	Kind       string   `json:"kind"`
	Title      string   `json:"title"`
	Content    string   `json:"content"`
	Labels     []string `json:"labels"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
	ArchivedAt *string  `json:"archived_at"`
}

// Thread is the replicated part of a conversation with an agent. NodeID is
// the node that runs it; only that node holds its messages and processes.
type Thread struct {
	ID             string  `json:"id"`
	NodeID         string  `json:"node_id"`
	ProjectID      *string `json:"project_id"`
	TaskID         *string `json:"task_id"`
	ParentID       *string `json:"parent_id"`
	Provider       string  `json:"provider"`
	Model          string  `json:"model"`
	Effort         string  `json:"effort"`
	RuntimeMode    string  `json:"runtime_mode"`
	Title          string  `json:"title"`
	Branch         string  `json:"branch"`
	Status         string  `json:"status"`
	PinnedAt       *string `json:"pinned_at"`
	ReadAt         *string `json:"read_at"`
	LastActivityAt *string `json:"last_activity_at"`
	SnoozedUntil   *string `json:"snoozed_until"`
	SettledAt      *string `json:"settled_at"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	ArchivedAt     *string `json:"archived_at"`
}

// Skill is a package of instructions for the harnesses. Targets says on which
// nodes and for which harnesses it is installed; empty lists mean all.
type Skill struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	BodyMD      string            `json:"body_md"`
	Files       map[string]string `json:"files"`
	Version     int64             `json:"version"`
	Targets     SkillTargets      `json:"targets"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
	ArchivedAt  *string           `json:"archived_at"`
}

type SkillTargets struct {
	Nodes     []string `json:"nodes"`
	Harnesses []string `json:"harnesses"`
}

// RepositoryLocation is where one node keeps a repository, published by that node.
type RepositoryLocation struct {
	RepositoryID string  `json:"repository_id"`
	NodeID       string  `json:"node_id"`
	Path         string  `json:"path"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
	ArchivedAt   *string `json:"archived_at"`
}

// SkillState is what one node really has of a skill, published by that node.
type SkillState struct {
	SkillID    string  `json:"skill_id"`
	NodeID     string  `json:"node_id"`
	Status     string  `json:"status"`
	Version    int64   `json:"version"`
	Message    string  `json:"message"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
	ArchivedAt *string `json:"archived_at"`
}

// Hit is a search result: a task or a memory, best match first.
type Hit struct {
	Entity string          `json:"entity"`
	ID     string          `json:"id"`
	Title  string          `json:"title"`
	Rank   float64         `json:"rank"`
	Record json.RawMessage `json:"record"`
}
