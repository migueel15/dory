package app

import (
	"context"

	"github.com/migueel15/dory/internal/data"
)

// Session composes what an agent needs when it starts working: the projects
// and, if its project is found, repositories, open tasks and memory titles.
type Session struct {
	Node     string
	Tasks    *Tasks
	Memories *Memories
	Projects *Projects
}

// TaskEntry is a task in an index.
type TaskEntry struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Column      string   `json:"column"`
	Branch      string   `json:"branch,omitempty"`
	IssueNumber *int64   `json:"issue_number,omitempty"`
	PRNumber    *int64   `json:"pr_number,omitempty"`
	Blockers    []string `json:"blockers,omitempty"`
}

// TaskIndex lists tasks without their description, so session_start and
// list_tasks stay small.
func TaskIndex(cards []Card) []TaskEntry {
	out := make([]TaskEntry, 0, len(cards))
	for _, c := range cards {
		out = append(out, TaskEntry{ID: c.ID, Title: c.Title, Column: c.Column, Branch: c.Branch, IssueNumber: c.IssueNumber, PRNumber: c.PRNumber, Blockers: c.Blockers})
	}
	return out
}

// Start answers session_start.
func (s *Session) Start(ctx context.Context, in FindInput) (map[string]any, error) {
	projects, err := s.Projects.List(ctx)
	if err != nil {
		return nil, err
	}
	global, err := s.Memories.Global(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"node_id": s.Node, "projects": projects, "memories": s.Memories.Index(global),
		"detail": "Memories and tasks are summaries: read a memory's content with recall and a task with get_task."}
	project, err := s.Projects.Find(ctx, projects, in)
	if err != nil || project == nil {
		out["project_match"] = "none"
		return out, err
	}
	repos, _ := s.Projects.Repositories(ctx, project.ID)
	memories, _ := s.Memories.OfProject(ctx, project.ID)
	tasks, err := s.Tasks.ListCards(ctx, ListInput{ProjectID: project.ID, Open: true})
	if err != nil {
		return nil, err
	}
	out["project"] = map[string]any{"project": project, "repositories": repos, "memories": s.Memories.Index(memories), "open_tasks": TaskIndex(tasks)}
	return out, nil
}

// Detail is what an agent needs of one task: the card, its plan and its
// dependencies.
func (a *Tasks) Detail(ctx context.Context, id string) (map[string]any, error) {
	c, err := a.Card(ctx, id)
	if err != nil {
		return nil, err
	}
	threads := make([]map[string]string, len(c.Threads))
	for i, t := range c.Threads {
		threads[i] = map[string]string{"id": t.ID, "title": t.Title, "status": t.Status, "node_id": t.NodeID}
	}
	task := map[string]any{"id": c.ID, "title": c.Title, "description": c.Description, "stage": c.Stage, "column": c.Column,
		"branch": c.Branch, "repo": c.Repo, "issue_number": c.IssueNumber, "pr_number": c.PRNumber, "pr_state": c.PRState,
		"project_id": c.ProjectID, "blockers": c.Blockers, "threads": threads}
	if c.Pull != nil {
		task["ci"], task["review_decision"] = c.Pull.CI, c.Pull.ReviewDecision
	}
	out := map[string]any{"task": task}
	if plan, err := a.Plans.Document(ctx, id); err == nil && plan.ID != "" {
		out["plan"] = map[string]any{"body_md": plan.BodyMD, "revision": plan.Revision, "accepted": plan.AcceptedAt != nil}
	}
	if deps, _ := a.Store.Dependencies().List(ctx, data.Filter{Fields: map[string]any{"task_id": id}}); len(deps) > 0 {
		out["dependencies"] = deps
	}
	return out, nil
}
