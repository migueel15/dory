package service

import "time"

type Record struct {
	ID         string     `json:"id"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	ArchivedAt *time.Time `json:"archived_at"`
}
type Project struct {
	Record
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Stack       []string `json:"stack"`
}
type Repository struct {
	Record
	ProjectID     string `json:"project_id"`
	RemoteUrl     string `json:"remote_url"`
	DefaultBranch string `json:"default_branch"`
}
type Environment struct {
	Record
	Name            string   `json:"name"`
	Os              string   `json:"os"`
	Access          string   `json:"access"`
	AgentsAvailable []string `json:"agents_available"`
	Notes           string   `json:"notes"`
}
type RepositoryLocation struct {
	Record
	RepositoryID  string `json:"repository_id"`
	EnvironmentID string `json:"environment_id"`
	Path          string `json:"path"`
}
type T3Instance struct {
	Record
	EnvironmentID string `json:"environment_id"`
	BaseUrl       string `json:"base_url"`
	TokenEnv      string `json:"token_env"`
}
type ItemThread struct {
	Record
	ItemID     string `json:"item_id"`
	InstanceID string `json:"instance_id"`
	ThreadID   string `json:"thread_id"`
	Role       string `json:"role"`
}
type IssueThread struct {
	Kind string `json:"kind"`
	Record
	ProjectID  string `json:"project_id"`
	Repo       string `json:"repo"`
	Number     int    `json:"number"`
	IssueURL   string `json:"issue_url"`
	Title      string `json:"title"`
	InstanceID string `json:"instance_id"`
	ThreadID   string `json:"thread_id"`
	Model      string `json:"model"`
	Effort     string `json:"effort"`
}
type Feature struct {
	Record
	ProjectID string   `json:"project_id"`
	Title     string   `json:"title"`
	BodyMd    string   `json:"body_md"`
	CodeRefs  []string `json:"code_refs"`
	Status    string   `json:"status"`
}
type Item struct {
	Record
	ProjectID     *string  `json:"project_id"`
	FeatureID     *string  `json:"feature_id"`
	RawText       string   `json:"raw_text"`
	Title         string   `json:"title"`
	BodyMd        string   `json:"body_md"`
	Progress      string   `json:"progress"`
	Refs          []string `json:"refs"`
	Tags          []string `json:"tags"`
	Status        string   `json:"status"`
	BlockedReason string   `json:"blocked_reason"`
	Priority      int32    `json:"priority"`
	Source        string   `json:"source"`
	// TitleSource is who wrote the title: pending (being generated), ai, user or fallback (the generation failed).
	TitleSource string `json:"title_source"`
}

// PlanSections are the fixed sections of an item plan, in reading order.
type PlanSections struct {
	Goal         string `json:"goal"`
	Scope        string `json:"scope"`
	Where        string `json:"where"`
	Design       string `json:"design"`
	Requirements string `json:"requirements"`
	Validation   string `json:"validation"`
	Risks        string `json:"risks"`
}

// ItemPlan is one immutable version of an item plan. Version is assigned by
// the database; only AcceptedAt changes once the version exists.
type ItemPlan struct {
	Record
	ItemID        string       `json:"item_id"`
	Version       int          `json:"version"`
	Sections      PlanSections `json:"sections"`
	OpenQuestions []string     `json:"open_questions"`
	Author        string       `json:"author"`
	AcceptedAt    *time.Time   `json:"accepted_at"`
}
type Memory struct {
	Record
	Scope   string   `json:"scope"`
	ScopeID *string  `json:"scope_id"`
	Kind    string   `json:"kind"`
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}
type Link struct {
	Record
	FromID   string `json:"from_id"`
	FromType string `json:"from_type"`
	ToID     string `json:"to_id"`
	ToType   string `json:"to_type"`
	Kind     string `json:"kind"`
}
type User struct {
	Record
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
}
type Session struct {
	Record
	UserID     string    `json:"user_id"`
	SecretHash string    `json:"secret_hash"`
	ExpiresAt  time.Time `json:"expires_at"`
}
type Token struct {
	Record
	UserID      string     `json:"user_id"`
	Name        string     `json:"name"`
	SecretHash  string     `json:"secret_hash"`
	Permissions []string   `json:"permissions"`
	ExpiresAt   *time.Time `json:"expires_at"`
}
