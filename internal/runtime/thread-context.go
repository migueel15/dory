package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/migueel15/dory/internal/data"
)

// threadContext is the short card of a thread that travels with the session
// guidance: the agent knows its project, repository and task without asking
// Dory for them. It is rebuilt every turn, so a task linked or unlinked
// during the conversation shows up on the next turn.
func (m *Manager) threadContext(ctx context.Context, t data.Thread, dir string) string {
	if m.store == nil {
		return ""
	}
	lines := []string{"## This thread", fmt.Sprintf("- Thread %s, node %s.", t.ID, m.node)}
	if dir != "" {
		line := "- Folder " + dir
		if t.Branch != "" {
			line += ", branch " + t.Branch
		}
		lines = append(lines, line+".")
	}
	var task *data.Task
	if t.TaskID != nil {
		if got, err := m.store.Tasks().Get(ctx, *t.TaskID); err == nil {
			task = &got
		}
	}
	projectID := ""
	if t.ProjectID != nil {
		projectID = *t.ProjectID
	} else if task != nil && task.ProjectID != nil {
		projectID = *task.ProjectID
	}
	if projectID != "" {
		if p, err := m.store.Projects().Get(ctx, projectID); err == nil {
			lines = append(lines, fmt.Sprintf("- Project %s (id %s).", p.Slug, p.ID))
		}
		if r, ok := m.threadRepository(ctx, projectID, task); ok {
			lines = append(lines, fmt.Sprintf("- Repository %s, default branch %s.", r.RemoteURL, r.DefaultBranch))
		}
	} else {
		lines = append(lines, "- No project: call session_start if the work needs one.")
	}
	if task == nil {
		lines = append(lines, "- No task linked. When the work needs one, search for it, then ensure_thread_task.")
		return strings.Join(lines, "\n")
	}
	line := fmt.Sprintf("- Task %s: %s. Stage %s", task.ID, task.Title, task.Stage)
	if task.IssueNumber != nil {
		line += fmt.Sprintf(", issue #%d", *task.IssueNumber)
	}
	if task.PRNumber != nil {
		line += fmt.Sprintf(", pull request #%d", *task.PRNumber)
	}
	if task.Branch != "" {
		line += ", branch " + task.Branch
	}
	return strings.Join(append(lines, line+". Read its description and plan with get_task when you need them."), "\n")
}

// threadRepository is the task's repository, or the project's only one.
func (m *Manager) threadRepository(ctx context.Context, projectID string, task *data.Task) (data.Repository, bool) {
	if task != nil && task.RepositoryID != nil {
		r, err := m.store.Repositories().Get(ctx, *task.RepositoryID)
		return r, err == nil
	}
	repos, err := m.store.Repositories().List(ctx, data.Filter{Fields: map[string]any{"project_id": projectID}, Limit: 2})
	if err != nil || len(repos) != 1 {
		return data.Repository{}, false
	}
	return repos[0], true
}
