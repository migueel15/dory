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
	ProjectID *string  `json:"project_id"`
	RawText   string   `json:"raw_text"`
	Title     string   `json:"title"`
	BodyMd    string   `json:"body_md"`
	Progress  string   `json:"progress"`
	Tags      []string `json:"tags"`
	Status    string   `json:"status"`
	Priority  int32    `json:"priority"`
	Source    string   `json:"source"`
}
type Memory struct {
	Record
	Scope   string   `json:"scope"`
	ScopeID *string  `json:"scope_id"`
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
