package mcp

import (
	"context"
	"strings"

	"github.com/migueel15/dory/internal/app"
	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/github"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type sessionInput struct {
	Project   string `json:"project,omitempty" jsonschema:"Project id or slug"`
	RemoteURL string `json:"remote_url,omitempty" jsonschema:"Git remote used to find the project when project is omitted"`
	Path      string `json:"path,omitempty" jsonschema:"Local checkout path used to find the project when the others are omitted"`
}

type taskListInput struct {
	ProjectID   string `json:"project_id,omitempty"`
	Source      string `json:"source,omitempty" jsonschema:"internal, github or all"`
	Scope       string `json:"scope,omitempty" jsonschema:"all, mine or mine_or_unassigned"`
	IssueNumber int64  `json:"issue_number,omitempty" jsonschema:"Only the tasks of this issue"`
	Open        bool   `json:"open,omitempty" jsonschema:"Leave out done and cancelled tasks"`
}

type idInput struct {
	ID     string `json:"id,omitempty" jsonschema:"Task UUID"`
	Repo   string `json:"repo,omitempty" jsonschema:"owner/repo for a GitHub issue"`
	Number int64  `json:"number,omitempty" jsonschema:"GitHub issue number"`
}

func resolveTaskID(ctx context.Context, store *data.Store, id, repo string, number int64) (string, error) {
	if id != "" {
		return id, nil
	}
	if repo == "" || number <= 0 {
		return "", apperr.New(apperr.Invalid, "id or repo + number required")
	}
	repos, err := store.Repositories().ListAll(ctx, data.Filter{})
	if err != nil {
		return "", err
	}
	for _, r := range repos {
		if name, ok := github.ParseRemote(r.RemoteURL); ok && strings.EqualFold(name, repo) {
			t, err := store.EnsureExternalTask(ctx, &r.ProjectID, r.ID, number)
			if err != nil {
				return "", err
			}
			return t.ID, nil
		}
	}
	return "", data.ErrNotFound.Msgf("repository not registered: %s", repo)
}

type taskInput struct {
	ID           string   `json:"id,omitempty" jsonschema:"Set to update an existing task"`
	ProjectID    string   `json:"project_id,omitempty"`
	RepositoryID string   `json:"repository_id,omitempty"`
	Title        string   `json:"title,omitempty"`
	Description  string   `json:"description,omitempty" jsonschema:"What to change and how to check it"`
	IssueNumber  int64    `json:"issue_number,omitempty"`
	PRNumber     int64    `json:"pr_number,omitempty"`
	Stage        string   `json:"stage,omitempty" jsonschema:"inbox, ready, done or cancelled. done only for a task without pull request or with a merged one; done and cancelled may archive its threads"`
	DependsOn    []string `json:"depends_on,omitempty" jsonschema:"Ids of tasks that must finish first"`
}

type dependencyInput struct {
	TaskID      string `json:"task_id" jsonschema:"The task that waits"`
	DependsOnID string `json:"depends_on_id" jsonschema:"The task that must finish first"`
}

func addTaskTools(srv *sdk.Server, s *Deps) {
	store := s.Store
	add(srv, "session_start", "Index of projects and, if found, the project's repositories, open tasks and memory titles. Only when you lack the project or need its backlog; read details with get_task and recall.", func(ctx context.Context, in sessionInput) (any, error) {
		return s.Session.Start(ctx, app.FindInput{Project: in.Project, RemoteURL: in.RemoteURL, Path: in.Path})
	})
	add(srv, "list_projects", "List projects with id, slug and name.", func(ctx context.Context, _ struct{}) (any, error) {
		return s.Projects.List(ctx)
	})
	add(srv, "list_tasks", "Index of tasks (internal and GitHub issues): id, source, title, column, reason, PR summary and blockers. Filter by project_id, source, scope or open.", func(ctx context.Context, in taskListInput) (any, error) {
		if in.IssueNumber != 0 {
			cards, err := s.Tasks.ListCards(ctx, app.ListInput{ProjectID: in.ProjectID, IssueNumber: in.IssueNumber, Open: in.Open})
			if err != nil {
				return nil, err
			}
			return app.TaskIndex(cards), nil
		}
		viewer := ""
		if s.GitHub != nil {
			viewer, _ = s.GitHub.Viewer(ctx)
		}
		cards, err := s.Tasks.BoardMulti(ctx, app.TaskFilter{
			ProjectID: in.ProjectID,
			Source:    in.Source,
			Scope:     in.Scope,
			Open:      in.Open,
			Viewer:    viewer,
		})
		if err != nil {
			return nil, err
		}
		return app.TaskIndex(cards), nil
	})
	add(srv, "get_task", "Read one task by id or repo+number: description, column, PRs, threads, current plan and dependencies.", func(ctx context.Context, in idInput) (any, error) {
		tid, err := resolveTaskID(ctx, store, in.ID, in.Repo, in.Number)
		if err != nil {
			return nil, err
		}
		return s.Tasks.Detail(ctx, tid)
	})
	add(srv, "set_task_state", "Change a task's state (open, done or cancelled, with optional reason for GitHub issues).", func(ctx context.Context, in struct {
		ID     string `json:"id,omitempty"`
		Repo   string `json:"repo,omitempty"`
		Number int64  `json:"number,omitempty"`
		State  string `json:"state" jsonschema:"open, done or cancelled"`
		Reason string `json:"reason,omitempty" jsonschema:"COMPLETED or NOT_PLANNED for GitHub issues"`
	}) (any, error) {
		tid, err := resolveTaskID(ctx, store, in.ID, in.Repo, in.Number)
		if err != nil {
			return nil, err
		}
		p, _, err := s.Tasks.ProviderFor(ctx, tid)
		if err != nil {
			return nil, err
		}
		return p.SetState(ctx, tid, app.TaskStateInput{State: in.State, Reason: in.Reason})
	})
	add(srv, "comment_task", "Add a comment to an external task (GitHub issue).", func(ctx context.Context, in struct {
		ID     string `json:"id,omitempty"`
		Repo   string `json:"repo,omitempty"`
		Number int64  `json:"number,omitempty"`
		Body   string `json:"body"`
	}) (any, error) {
		tid, err := resolveTaskID(ctx, store, in.ID, in.Repo, in.Number)
		if err != nil {
			return nil, err
		}
		p, _, err := s.Tasks.ProviderFor(ctx, tid)
		if err != nil {
			return nil, err
		}
		return p.Comment(ctx, tid, in.Body)
	})
	add(srv, "save_task", "Create a task, or change its title, description, issue, pull request or stage. stage done only without an unmerged pull request.", func(ctx context.Context, in taskInput) (any, error) {
		return s.Tasks.Save(ctx, app.SaveInput{ID: in.ID, ProjectID: in.ProjectID, RepositoryID: in.RepositoryID, Title: in.Title, Description: in.Description,
			IssueNumber: in.IssueNumber, PRNumber: in.PRNumber, Stage: in.Stage, DependsOn: in.DependsOn})
	})
	add(srv, "set_task_project", "Move a task to a project, or out of it with an empty project_id.", func(ctx context.Context, in struct {
		TaskID    string `json:"task_id"`
		ProjectID string `json:"project_id,omitempty" jsonschema:"Project id or slug; empty to unlink the task from its project"`
	}) (any, error) {
		return s.Projects.SetTaskProject(ctx, in.TaskID, in.ProjectID)
	})
	add(srv, "ensure_thread_task", "Get this conversation's task, creating and linking one if it has none. Use it before writing a plan. Safe to retry.", func(ctx context.Context, in struct {
		Title       string `json:"title" jsonschema:"Concise title for a new task"`
		Description string `json:"description,omitempty" jsonschema:"Description for a new task"`
		TaskID      string `json:"task_id,omitempty" jsonschema:"Link this conversation to this existing task instead of creating one"`
	}) (any, error) {
		id, _ := ctx.Value(threadKey{}).(string)
		if id == "" {
			return nil, apperr.New(apperr.Invalid, "ensure_thread_task only works inside a Dory thread")
		}
		if in.TaskID != "" {
			if _, err := store.LinkThread(ctx, id, in.TaskID, false); err != nil {
				return nil, err
			}
			return store.Tasks().Get(ctx, in.TaskID)
		}
		return store.EnsureThreadTask(ctx, id, in.Title, in.Description)
	})
	add(srv, "link_thread", "Link a thread (this one by default) to an existing task. replace moves it from another task.", func(ctx context.Context, in struct {
		ThreadID string `json:"thread_id,omitempty" jsonschema:"Thread to link; empty for this conversation"`
		TaskID   string `json:"task_id"`
		Replace  bool   `json:"replace,omitempty" jsonschema:"Move the thread when it is linked to another task"`
	}) (any, error) {
		id, err := threadID(ctx, in.ThreadID)
		if err != nil {
			return nil, err
		}
		return store.LinkThread(ctx, id, in.TaskID, in.Replace)
	})
	add(srv, "unlink_thread", "Unlink a thread (this one by default) from its task.", func(ctx context.Context, in struct {
		ThreadID string `json:"thread_id,omitempty" jsonschema:"Thread to unlink; empty for this conversation"`
	}) (any, error) {
		id, err := threadID(ctx, in.ThreadID)
		if err != nil {
			return nil, err
		}
		return store.UnlinkThread(ctx, id)
	})
	add(srv, "list_task_threads", "List the threads of a task with title, node and status.", func(ctx context.Context, in planReadInput) (any, error) {
		threads, err := store.Threads().ListAll(ctx, data.Filter{Fields: map[string]any{"task_id": in.TaskID}})
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, len(threads))
		for i, t := range threads {
			out[i] = map[string]any{"id": t.ID, "title": t.Title, "node_id": t.NodeID, "status": t.Status, "branch": t.Branch}
		}
		return out, nil
	})
	add(srv, "add_task_dependency", "Make a task wait until another finishes.", func(ctx context.Context, in dependencyInput) (any, error) {
		return store.AddDependency(ctx, in.TaskID, in.DependsOnID)
	})
	add(srv, "remove_task_dependency", "Remove a blocker from a task.", func(ctx context.Context, in dependencyInput) (any, error) {
		if err := store.RemoveDependency(ctx, in.TaskID, in.DependsOnID); err != nil {
			return nil, err
		}
		return map[string]any{"removed": true}, nil
	})
}
