// Package peerapi defines request and response types shared by the mesh client and API.
package peerapi

type CreateThreadRequest struct {
	ProjectID    *string `json:"project_id"`
	TaskID       *string `json:"task_id"`
	ParentID     *string `json:"parent_id"`
	OriginID     *string `json:"origin_id"`
	RepositoryID string  `json:"repository_id"`
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	Effort       string  `json:"effort"`
	RuntimeMode  string  `json:"runtime_mode"`
	Title        string  `json:"title"`
	Text         string  `json:"text"`
	Worktree     bool    `json:"worktree"`
	Branch       string  `json:"branch"`
	BaseBranch   string  `json:"base_branch"`
	WorktreePath string  `json:"worktree_path"`
}

type CreateThreadResponse struct {
	ThreadID string `json:"thread_id"`
}

type SendMessageRequest struct {
	Text  string   `json:"text"`
	Files []string `json:"files"`
	// SenderThreadID marks a message an agent of that thread wrote.
	SenderThreadID string `json:"sender_thread_id"`
}
