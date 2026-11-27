package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/mesh/peerapi"
)

// LaunchInput chooses how an implementation thread starts; Text replaces the
// generated first message.
type LaunchInput struct {
	Provider    string
	Model       string
	Effort      string
	RuntimeMode string
	BaseBranch  string
	Text        string
}

// Launch starts an implementation thread for a task on this node, in its own
// worktree and branch. It returns the thread id and the branch.
func (a *Tasks) Launch(ctx context.Context, taskID string, in LaunchInput) (threadID, branch string, err error) {
	t, u, err := a.EnsureTaskRow(ctx, taskID)
	if err != nil {
		return "", "", err
	}
	if t.Title == "" && u.Title != "" {
		t.Title = u.Title
	}
	if t.Description == "" && u.Description != "" {
		t.Description = u.Description
	}
	if pending, err := a.Plans.Pending(ctx, t.ID); err != nil {
		return "", "", err
	} else if pending {
		return "", "", apperr.New(apperr.PlanNotAccepted, "accept the task plan before implementing it")
	}
	fields := data.Fields{}
	if t.RepositoryID == nil && t.ProjectID != nil {
		repos, _ := a.Store.Repositories().List(ctx, data.Filter{Fields: map[string]any{"project_id": *t.ProjectID}})
		if len(repos) == 1 {
			t.RepositoryID = &repos[0].ID
			fields["repository_id"] = repos[0].ID
		}
	}
	if t.RepositoryID == nil {
		return "", "", apperr.New(apperr.NoRepository, "the task needs a repository")
	}
	if _, _, err := RepositoryCheckout(ctx, a.Store, t.ProjectID, *t.RepositoryID); err != nil {
		return "", "", err
	}
	if t.Branch == "" {
		t.Branch = TaskBranch(t)
		fields["branch"] = t.Branch
	}
	if in.BaseBranch == "" {
		repo, _ := a.Store.Repositories().Get(ctx, *t.RepositoryID)
		in.BaseBranch = repo.DefaultBranch
	}
	if t, err = a.Store.Tasks().PatchIf(ctx, t.ID, t.UpdatedAt, fields); err != nil {
		return "", "", err
	}
	prompt := in.Text
	if prompt == "" {
		prompt = a.Prompt(ctx, t)
	}
	worktree := true
	th, err := a.Threads.Create(ctx, CreateThread{CreateThreadRequest: peerapi.CreateThreadRequest{
		ProjectID: t.ProjectID, RepositoryID: *t.RepositoryID, TaskID: &t.ID, Provider: in.Provider, Model: in.Model,
		Effort: in.Effort, RuntimeMode: in.RuntimeMode, Title: t.Title, Text: prompt, Worktree: &worktree, Branch: t.Branch, BaseBranch: in.BaseBranch,
	}})
	if err != nil {
		return "", "", err
	}
	return th.ID, t.Branch, nil
}

// Prompt is the first message of an implementation thread.
func (a *Tasks) Prompt(ctx context.Context, t data.Task) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Implementa esta tarea de Dory (id %s) en la rama %s.\n\n# %s\n\n%s\n", t.ID, t.Branch, t.Title, t.Description)
	issue := ""
	if t.IssueNumber != nil {
		issue = fmt.Sprint(*t.IssueNumber)
		fmt.Fprintf(&b, "\nIssue de GitHub: #%d.\n", *t.IssueNumber)
	}
	plans, _ := a.Store.Plans().List(ctx, data.Filter{Fields: map[string]any{"task_id": t.ID}})
	var accepted *data.Plan
	for i := range plans {
		if plans[i].AcceptedAt != nil && (accepted == nil || plans[i].Version > accepted.Version) {
			accepted = &plans[i]
		}
	}
	if accepted != nil {
		fmt.Fprintf(&b, "\nPlan aceptado:\n\n%s\n", PlanMarkdown(*accepted))
	}
	return AutoMessage("implement", map[string]string{"task": t.ID, "title": t.Title, "branch": t.Branch, "issue": issue}, b.String(), "")
}
