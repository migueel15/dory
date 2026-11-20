// Package mcp exposes Dory to the agents of this node over streamable HTTP.
// Agents read everything they need about the work in progress; they write
// only what is theirs: memories, plans, questions and tasks. The kanban
// column is derived; the only state they set is the stage of a task, with
// the same rules as the interface.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/migueel15/dory/internal/api"
	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/oplog"
	"github.com/migueel15/dory/internal/runtime"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type threadKey struct{}

// add registers a tool. Tools whose name starts with a read verb need the
// read scope; the rest need write. Results are JSON text.
func add[I any](server *sdk.Server, name, description string, fn func(context.Context, I) (any, error)) {
	sdk.AddTool(server, &sdk.Tool{Name: name, Description: description}, func(ctx context.Context, _ *sdk.CallToolRequest, in I) (*sdk.CallToolResult, any, error) {
		scope := "write"
		for _, prefix := range []string{"list_", "get_", "search", "recall", "session_start"} {
			if strings.HasPrefix(name, prefix) {
				scope = "read"
			}
		}
		if !auth.Allowed(ctx, scope) {
			return nil, nil, fmt.Errorf("permiso %q requerido", scope)
		}
		value, err := fn(ctx, in)
		if err != nil {
			return nil, nil, err
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, nil, err
		}
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(raw)}}}, nil, nil
	})
}

type sessionInput struct {
	Project   string `json:"project,omitempty" jsonschema:"Project id or slug"`
	RemoteURL string `json:"remote_url,omitempty" jsonschema:"Git remote used to find the project when project is omitted"`
	Path      string `json:"path,omitempty" jsonschema:"Local checkout path used to find the project when the others are omitted"`
}

type taskListInput struct {
	ProjectID   string `json:"project_id,omitempty"`
	IssueNumber int64  `json:"issue_number,omitempty" jsonschema:"Only the tasks of this issue"`
	Open        bool   `json:"open,omitempty" jsonschema:"Leave out done and cancelled tasks"`
}

type idInput struct {
	ID string `json:"id"`
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

type planReadInput struct {
	TaskID string `json:"task_id"`
}

type planWriteInput struct {
	TaskID   string `json:"task_id"`
	BodyMD   string `json:"body_md" jsonschema:"The whole plan as Markdown"`
	Revision *int64 `json:"revision,omitempty" jsonschema:"Revision returned by plan_read; the save fails if the plan changed since"`
}

type planEditInput struct {
	TaskID   string `json:"task_id"`
	OldText  string `json:"old_text" jsonschema:"Exact text of the plan to replace; it must appear once"`
	NewText  string `json:"new_text"`
	Revision *int64 `json:"revision,omitempty" jsonschema:"Revision returned by plan_read; the edit fails if the plan changed since"`
}

type memoryInput struct {
	ID      string   `json:"id,omitempty" jsonschema:"Set to update an existing memory"`
	Scope   string   `json:"scope,omitempty" jsonschema:"global, project or node"`
	ScopeID string   `json:"scope_id,omitempty" jsonschema:"Project id or node id; empty for global"`
	Kind    string   `json:"kind,omitempty" jsonschema:"decision, lesson or context"`
	Title   string   `json:"title,omitempty"`
	Content string   `json:"content,omitempty"`
	Labels  []string `json:"labels,omitempty"`
}

type recallInput struct {
	Query     string   `json:"query,omitempty" jsonschema:"Words to search; empty lists by filters"`
	ProjectID string   `json:"project_id,omitempty"`
	Labels    []string `json:"labels,omitempty" jsonschema:"Memories that carry all these labels"`
	Kind      string   `json:"kind,omitempty"`
}

type askInput struct {
	Question  string             `json:"question,omitempty" jsonschema:"A single free question; prefer questions"`
	Questions []runtime.Question `json:"questions,omitempty" jsonschema:"One or more questions, each with a short header and 2-4 options; the user may also answer freely"`
}

// Handler serves the tools. The caller authenticates the request; a thread
// passes its id as ?thread= so questions reach the right conversation.
func Handler(s *api.Server) http.Handler {
	srv := sdk.NewServer(&sdk.Implementation{Name: "dory", Version: "2.0.0"}, nil)
	store := s.Store

	add(srv, "session_start", "Load the context to work on a project: global memories, projects and, when found, the project with its repositories, open tasks and memories.", func(ctx context.Context, in sessionInput) (any, error) {
		return sessionStart(ctx, s, in)
	})
	add(srv, "list_projects", "List projects.", func(ctx context.Context, _ struct{}) (any, error) {
		return store.Projects().List(ctx, data.Filter{Limit: 1000})
	})
	add(srv, "list_tasks", "List tasks with their kanban column, branch, pull request, threads and blockers.", func(ctx context.Context, in taskListInput) (any, error) {
		return listTasks(ctx, s, in)
	})
	add(srv, "get_task", "Get a task with its column, threads, pull request, plans and dependencies.", func(ctx context.Context, in idInput) (any, error) {
		t, err := store.Tasks().Get(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		cards, err := s.Cards(ctx, []data.Task{t})
		if err != nil {
			return nil, err
		}
		plans, _ := store.Plans().List(ctx, data.Filter{Fields: map[string]any{"task_id": t.ID}})
		deps, _ := store.Dependencies().List(ctx, data.Filter{Fields: map[string]any{"task_id": t.ID}})
		return map[string]any{"task": cards[0], "plans": plans, "dependencies": deps}, nil
	})
	add(srv, "save_task", "Create a task, or update the title, description, issue, pull request or stage of one. Tasks created from one issue share its number; depends_on marks which must finish first. Use stage done for work integrated straight into the default branch; it is refused while the task has a pull request that is not merged.", func(ctx context.Context, in taskInput) (any, error) {
		return saveTask(ctx, s, in)
	})
	add(srv, "set_task_project", "Link a task to a project, or unlink it by leaving project_id empty. Use it when the user asks to put a task in a project or take it out. Moving it to another project clears its repository, which is set again when the project has only one.", func(ctx context.Context, in struct {
		TaskID    string `json:"task_id"`
		ProjectID string `json:"project_id,omitempty" jsonschema:"Project id or slug; empty to unlink the task from its project"`
	}) (any, error) {
		return setTaskProject(ctx, store, in.TaskID, in.ProjectID)
	})
	add(srv, "ensure_thread_task", "When planning in this conversation, reuse its linked task or create and link one atomically. Safe to retry. Supply a concise title and description only when a task is needed; use plan_read and plan_write or plan_edit for its plan. Only the user accepts the plan.", func(ctx context.Context, in struct {
		Title       string `json:"title" jsonschema:"Concise title for a new task"`
		Description string `json:"description,omitempty" jsonschema:"Description for a new task"`
		TaskID      string `json:"task_id,omitempty" jsonschema:"Link this conversation to this existing task instead of creating one"`
	}) (any, error) {
		id, _ := ctx.Value(threadKey{}).(string)
		if id == "" {
			return nil, errors.New("ensure_thread_task only works inside a Dory thread")
		}
		if in.TaskID != "" {
			if _, err := store.LinkThread(ctx, id, in.TaskID, false); err != nil {
				return nil, err
			}
			return store.Tasks().Get(ctx, in.TaskID)
		}
		return store.EnsureThreadTask(ctx, id, in.Title, in.Description)
	})
	add(srv, "link_thread", "Link a thread (this conversation by default) to an existing task of its project. Linking it to the same task again does nothing; moving it from another task needs replace.", func(ctx context.Context, in struct {
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
	add(srv, "unlink_thread", "Remove the task of a thread (this conversation by default).", func(ctx context.Context, in struct {
		ThreadID string `json:"thread_id,omitempty" jsonschema:"Thread to unlink; empty for this conversation"`
	}) (any, error) {
		id, err := threadID(ctx, in.ThreadID)
		if err != nil {
			return nil, err
		}
		return store.UnlinkThread(ctx, id)
	})
	add(srv, "list_task_threads", "List the threads linked to a task with their title, node and status.", func(ctx context.Context, in planReadInput) (any, error) {
		threads, err := store.Threads().List(ctx, data.Filter{Fields: map[string]any{"task_id": in.TaskID}, Limit: 1000})
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, len(threads))
		for i, t := range threads {
			out[i] = map[string]any{"id": t.ID, "title": t.Title, "node_id": t.NodeID, "status": t.Status, "branch": t.Branch}
		}
		return out, nil
	})
	add(srv, "add_task_dependency", "Mark that a task must wait until another finishes. Refuses a task that depends on itself and cycles.", func(ctx context.Context, in dependencyInput) (any, error) {
		return store.AddDependency(ctx, in.TaskID, in.DependsOnID)
	})
	add(srv, "remove_task_dependency", "Remove a task from the blockers of another.", func(ctx context.Context, in dependencyInput) (any, error) {
		if err := store.RemoveDependency(ctx, in.TaskID, in.DependsOnID); err != nil {
			return nil, err
		}
		return map[string]any{"removed": true}, nil
	})
	add(srv, "plan_read", "Read the plan of a task: one Markdown document that the user and the agents edit. Returns body_md and its revision.", func(ctx context.Context, in planReadInput) (any, error) {
		return s.PlanDocument(ctx, in.TaskID)
	})
	add(srv, "plan_write", "Write the whole plan of a task as Markdown, to start it. To change an existing plan use plan_edit; rewriting it needs the revision from plan_read. Only the user accepts a plan; changing it waits for a new acceptance.", func(ctx context.Context, in planWriteInput) (any, error) {
		// Rewriting an existing plan blind would drop what the user is
		// writing, so it needs the revision the agent read.
		if in.Revision == nil {
			if cur, err := s.PlanDocument(ctx, in.TaskID); err != nil {
				return nil, err
			} else if cur.Revision > 0 {
				return nil, errors.New("the plan already exists: change parts of it with plan_edit, or pass the revision from plan_read to rewrite it")
			}
		}
		return s.WritePlan(ctx, in.TaskID, in.BodyMD, revision(in.Revision), "agent")
	})
	add(srv, "plan_edit", "Replace one exact piece of the plan of a task, so the user sees it change in place. old_text must appear once; read the plan again if the edit fails.", func(ctx context.Context, in planEditInput) (any, error) {
		return s.EditPlan(ctx, in.TaskID, in.OldText, in.NewText, revision(in.Revision), "agent")
	})
	add(srv, "remember", "Store or update a memory. Scope global, project (scope_id = project id) or node (scope_id = node id).", func(ctx context.Context, in memoryInput) (any, error) {
		return remember(ctx, store, in)
	})
	add(srv, "recall", "Find memories by text, project, labels or kind.", func(ctx context.Context, in recallInput) (any, error) {
		return recall(ctx, store, in)
	})
	add(srv, "search", "Search tasks and memories by text.", func(ctx context.Context, in struct {
		Query string `json:"query"`
	}) (any, error) {
		return store.Search(ctx, in.Query, 20)
	})
	add(srv, "ask_user", "Ask the user one or more questions, each with options to pick (multi_select for several). The call waits until the user answers in the chat and returns each question with its answer.", func(ctx context.Context, in askInput) (any, error) {
		id, _ := ctx.Value(threadKey{}).(string)
		if id == "" {
			return nil, errors.New("ask_user only works inside a Dory thread")
		}
		qs := in.Questions
		if in.Question != "" {
			qs = append([]runtime.Question{{Question: in.Question}}, qs...)
		}
		if len(qs) == 0 {
			return nil, errors.New("ask_user needs a question")
		}
		answers, err := s.Runtime.Ask(ctx, id, qs)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, len(qs))
		for i, q := range qs {
			var a []string
			if i < len(answers) {
				a = answers[i]
			}
			out[i] = map[string]any{"question": q.Question, "answer": a}
		}
		return map[string]any{"answers": out}, nil
	})

	h := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return srv }, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := r.URL.Query().Get("thread"); id != "" {
			r = r.WithContext(context.WithValue(r.Context(), threadKey{}, id))
		}
		h.ServeHTTP(w, r)
	})
}

func sessionStart(ctx context.Context, s *api.Server, in sessionInput) (any, error) {
	store := s.Store
	projects, err := store.Projects().List(ctx, data.Filter{Limit: 1000})
	if err != nil {
		return nil, err
	}
	global, err := store.Memories().List(ctx, data.Filter{Fields: map[string]any{"scope": "global"}, Limit: 1000})
	if err != nil {
		return nil, err
	}
	out := map[string]any{"node_id": s.Node.ID, "projects": projects, "memories": global}
	project, err := findProject(ctx, store, projects, in)
	if err != nil || project == nil {
		out["project_match"] = "none"
		return out, err
	}
	repos, _ := store.Repositories().List(ctx, data.Filter{Fields: map[string]any{"project_id": project.ID}})
	memories, _ := store.Memories().List(ctx, data.Filter{Fields: map[string]any{"scope": "project", "scope_id": project.ID}, Limit: 1000})
	tasks, err := listTasks(ctx, s, taskListInput{ProjectID: project.ID, Open: true})
	if err != nil {
		return nil, err
	}
	out["project"] = map[string]any{"project": project, "repositories": repos, "memories": memories, "open_tasks": tasks}
	return out, nil
}

func findProject(ctx context.Context, store *data.Store, projects []data.Project, in sessionInput) (*data.Project, error) {
	for i, p := range projects {
		if in.Project != "" && (p.ID == in.Project || p.Slug == in.Project) {
			return &projects[i], nil
		}
	}
	remote := in.RemoteURL
	if remote == "" && in.Path != "" {
		paths, err := store.RepositoryPaths(ctx)
		if err != nil {
			return nil, err
		}
		for id, path := range paths {
			if path == in.Path || strings.HasPrefix(in.Path, path+"/") {
				if r, err := store.Repositories().Get(ctx, id); err == nil {
					remote = r.RemoteURL
				}
			}
		}
	}
	if remote == "" {
		return nil, nil
	}
	r, err := store.Repositories().Get(ctx, oplog.RepositoryID(remote))
	if errors.Is(err, data.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for i, p := range projects {
		if p.ID == r.ProjectID {
			return &projects[i], nil
		}
	}
	return nil, nil
}

func listTasks(ctx context.Context, s *api.Server, in taskListInput) ([]api.Card, error) {
	fields := map[string]any{}
	if in.ProjectID != "" {
		fields["project_id"] = in.ProjectID
	}
	if in.IssueNumber != 0 {
		fields["issue_number"] = in.IssueNumber
	}
	tasks, err := s.Store.Tasks().List(ctx, data.Filter{Fields: fields, Limit: 1000})
	if err != nil {
		return nil, err
	}
	cards, err := s.Cards(ctx, tasks)
	if err != nil {
		return nil, err
	}
	if in.Open {
		cards = slices.DeleteFunc(cards, func(c api.Card) bool { return c.Column == "done" || c.Column == "cancelled" })
	}
	return cards, nil
}

func saveTask(ctx context.Context, s *api.Server, in taskInput) (data.Task, error) {
	store := s.Store
	var t data.Task
	if in.ID != "" {
		var err error
		if t, err = store.Tasks().Get(ctx, in.ID); err != nil {
			return t, err
		}
	} else if in.Title == "" {
		return t, fmt.Errorf("%w: title is required", data.ErrInvalid)
	}
	set := func(dst **string, v string) {
		if v != "" {
			*dst = &v
		}
	}
	set(&t.ProjectID, in.ProjectID)
	set(&t.RepositoryID, in.RepositoryID)
	if in.Title != "" {
		t.Title = in.Title
	}
	if in.Description != "" {
		t.Description = in.Description
	}
	if in.IssueNumber != 0 {
		t.IssueNumber = &in.IssueNumber
	}
	if in.PRNumber != 0 {
		t.PRNumber = &in.PRNumber
	}
	if in.Stage != "" {
		t.Stage = in.Stage
	}
	if t.RepositoryID == nil && t.ProjectID != nil {
		if repos, _ := store.Repositories().List(ctx, data.Filter{Fields: map[string]any{"project_id": *t.ProjectID}}); len(repos) == 1 {
			t.RepositoryID = &repos[0].ID
		}
	}
	t, err := s.SaveTask(ctx, t)
	if err != nil {
		return t, err
	}
	for _, dep := range in.DependsOn {
		if _, err = store.AddDependency(ctx, t.ID, dep); err != nil {
			return t, err
		}
	}
	return t, nil
}

func setTaskProject(ctx context.Context, store *data.Store, taskID, project string) (data.Task, error) {
	t, err := store.Tasks().Get(ctx, taskID)
	if err != nil {
		return t, err
	}
	var next *string
	if project != "" {
		p, err := store.Projects().Get(ctx, project)
		if errors.Is(err, data.ErrNotFound) {
			projects, lerr := store.Projects().List(ctx, data.Filter{Limit: 1000})
			if lerr != nil {
				return t, lerr
			}
			for i := range projects {
				if projects[i].Slug == project {
					p, err = projects[i], nil
					break
				}
			}
		}
		if err != nil {
			return t, err
		}
		if p.ID == "" {
			return t, fmt.Errorf("%w: project %s does not exist", data.ErrInvalid, project)
		}
		next = &p.ID
	}
	if (t.ProjectID == nil) == (next == nil) && (next == nil || *t.ProjectID == *next) {
		return t, nil
	}
	t.ProjectID, t.RepositoryID = next, nil
	if next != nil {
		if repos, _ := store.Repositories().List(ctx, data.Filter{Fields: map[string]any{"project_id": *next}}); len(repos) == 1 {
			t.RepositoryID = &repos[0].ID
		}
	}
	return store.Tasks().Put(ctx, t)
}

func remember(ctx context.Context, store *data.Store, in memoryInput) (data.Memory, error) {
	var m data.Memory
	if in.ID != "" {
		var err error
		if m, err = store.Memories().Get(ctx, in.ID); err != nil {
			return m, err
		}
	} else {
		m.Scope = "global"
	}
	if in.Scope != "" {
		m.Scope, m.ScopeID = in.Scope, in.ScopeID
	}
	if in.Kind != "" {
		m.Kind = in.Kind
	}
	if in.Title != "" {
		m.Title = in.Title
	}
	if in.Content != "" {
		m.Content = in.Content
	}
	if in.Labels != nil {
		m.Labels = in.Labels
	}
	if m.Content == "" {
		return m, fmt.Errorf("%w: content is required", data.ErrInvalid)
	}
	return store.Memories().Put(ctx, m)
}

func recall(ctx context.Context, store *data.Store, in recallInput) ([]data.Memory, error) {
	var memories []data.Memory
	if in.Query != "" {
		hits, err := store.Search(ctx, in.Query, 100)
		if err != nil {
			return nil, err
		}
		for _, h := range hits {
			if h.Entity != "memory" {
				continue
			}
			var m data.Memory
			if json.Unmarshal(h.Record, &m) == nil {
				memories = append(memories, m)
			}
		}
	} else {
		var err error
		if memories, err = store.Memories().List(ctx, data.Filter{Limit: 1000}); err != nil {
			return nil, err
		}
	}
	return slices.DeleteFunc(memories, func(m data.Memory) bool {
		if in.ProjectID != "" && (m.Scope != "project" || m.ScopeID != in.ProjectID) {
			return true
		}
		if in.Kind != "" && m.Kind != in.Kind {
			return true
		}
		for _, l := range in.Labels {
			if !slices.Contains(m.Labels, l) {
				return true
			}
		}
		return false
	}), nil
}

// threadID returns the given thread or the one of this conversation.
func threadID(ctx context.Context, id string) (string, error) {
	if id == "" {
		id, _ = ctx.Value(threadKey{}).(string)
	}
	if id == "" {
		return "", errors.New("thread_id is required outside a Dory thread")
	}
	return id, nil
}

// revision is the base revision of a plan save; -1 skips the check.
func revision(r *int64) int64 {
	if r == nil {
		return -1
	}
	return *r
}
