package mcp

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/migueel15/dory/internal/api"
	"github.com/migueel15/dory/internal/app"
	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/mesh"
	"github.com/migueel15/dory/internal/mesh/peerapi"
	"github.com/migueel15/dory/internal/runtime"
)

// resultChars caps each message an agent reads from another thread.
const resultChars = 4000

// caller is the thread whose agent makes the call, or "" outside a thread.
func caller(ctx context.Context) string {
	if id := auth.AgentThread(ctx); id != "" {
		return id
	}
	id, _ := ctx.Value(threadKey{}).(string)
	return id
}

// target is the thread a control tool acts on: any thread, this one by default.
func target(ctx context.Context, id string) (string, error) {
	if id = strings.TrimSpace(id); id != "" {
		return id, nil
	}
	if id = caller(ctx); id == "" {
		return "", apperr.New(apperr.Invalid, "thread_id is required outside a Dory thread")
	}
	return id, nil
}

type threadListInput struct {
	ProjectID       string `json:"project_id,omitempty"`
	TaskID          string `json:"task_id,omitempty"`
	ParentID        string `json:"parent_id,omitempty" jsonschema:"Only the children of this thread"`
	Status          string `json:"status,omitempty" jsonschema:"working, idle, waiting or error"`
	Query           string `json:"query,omitempty" jsonschema:"Words of the title"`
	IncludeArchived bool   `json:"include_archived,omitempty"`
	Limit           int    `json:"limit,omitempty" jsonschema:"Default 20"`
}

type threadConfigureInput struct {
	ThreadID     string  `json:"thread_id,omitempty" jsonschema:"Thread to change; empty for this conversation"`
	Provider     *string `json:"provider,omitempty" jsonschema:"Harness id from list_models, e.g. claude or codex; needs model"`
	Model        *string `json:"model,omitempty" jsonschema:"Model slug or name from list_models"`
	Effort       *string `json:"effort,omitempty" jsonschema:"Reasoning effort offered by the model"`
	ContinueWith string  `json:"continue_with,omitempty" jsonschema:"Message that starts the next turn with the new selection"`
}

func addThreadTools(srv *sdkServer, s *api.Server) {
	m := s.Runtime
	add(srv, "list_models", "List providers, models and efforts of this node. Use it to turn \"haiku\", \"opus\" or \"codex\" into provider and model. A model is accepted by slug, display name (\"Sonnet 5.5\") or resolved id (\"claude-sonnet-5-5\").", func(ctx context.Context, in struct {
		Provider string `json:"provider,omitempty"`
	}) (any, error) {
		out := []map[string]any{}
		for _, st := range m.Providers(ctx) {
			if !st.Installed || (in.Provider != "" && st.ID != in.Provider) {
				continue
			}
			out = append(out, map[string]any{"provider": st.ID, "name": st.Name, "enabled": st.Enabled, "logged_in": st.LoggedIn, "models": st.Models})
		}
		return out, nil
	})
	add(srv, "list_threads", "Find threads by project, task, parent, status or title words. Use it when the user names another thread.", func(ctx context.Context, in threadListInput) (any, error) {
		fields := map[string]any{}
		for k, v := range map[string]string{"project_id": in.ProjectID, "task_id": in.TaskID, "parent_id": in.ParentID, "status": in.Status} {
			if v != "" {
				fields[k] = v
			}
		}
		threads, err := s.Store.Threads().List(ctx, data.Filter{Fields: fields, IncludeArchived: in.IncludeArchived, Limit: 1000})
		if err != nil {
			return nil, err
		}
		words := strings.Fields(strings.ToLower(in.Query))
		threads = slices.DeleteFunc(threads, func(t data.Thread) bool {
			title := strings.ToLower(t.Title)
			for _, w := range words {
				if !strings.Contains(title, w) {
					return true
				}
			}
			return false
		})
		slices.SortFunc(threads, func(a, b data.Thread) int { return strings.Compare(activity(b), activity(a)) })
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		out := []map[string]any{}
		for _, t := range threads[:min(limit, len(threads))] {
			out = append(out, threadSummary(ctx, m, t))
		}
		return out, nil
	})
	add(srv, "get_thread", "Read a thread (this one by default): model, status, queue, children and last messages.", func(ctx context.Context, in struct {
		ThreadID string `json:"thread_id,omitempty"`
		Messages int    `json:"messages,omitempty" jsonschema:"Last visible messages to return; default 5"`
	}) (any, error) {
		id, err := target(ctx, in.ThreadID)
		if err != nil {
			return nil, err
		}
		t, err := s.Store.Threads().Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out := threadSummary(ctx, m, t)
		children, _ := s.Store.Threads().List(ctx, data.Filter{Fields: map[string]any{"parent_id": id}, Limit: 100})
		kids := make([]map[string]any, len(children))
		for i, c := range children {
			kids[i] = threadSummary(ctx, m, c)
		}
		out["children"] = kids
		if t.NodeID != s.Node.ID {
			out["messages"] = []any{}
			return out, nil
		}
		out["running"], out["queue_length"], out["pending_settings"] = m.Running(id), len(m.Queue(id)), m.Pending(id)
		msgs, err := m.Messages(ctx, id)
		if err != nil {
			return nil, err
		}
		n := in.Messages
		if n <= 0 {
			n = 5
		}
		visible := []map[string]any{}
		for _, msg := range msgs {
			if msg.Role != "user" && msg.Role != "assistant" {
				continue
			}
			visible = append(visible, map[string]any{"role": msg.Role, "text": clip(msg.Text), "created_at": msg.CreatedAt})
			if msg.Role == "assistant" {
				out["last_result"] = clip(msg.Text)
			}
		}
		out["messages"] = visible[max(0, len(visible)-n):]
		return out, nil
	})
	add(srv, "thread_configure", "Read or change a thread's provider, model or effort (this one by default), e.g. \"cámbiate a haiku\". Your own change applies after this turn; continue_with starts the next one.", func(ctx context.Context, in threadConfigureInput) (any, error) {
		id, err := target(ctx, in.ThreadID)
		if err != nil {
			return nil, err
		}
		settings := runtime.ThreadSettings{Provider: in.Provider, Model: in.Model, Effort: in.Effort}
		if in.Provider == nil && in.Model == nil && in.Effort == nil {
			if in.ContinueWith != "" {
				return nil, apperr.New(apperr.Invalid, "continue_with needs a provider, model or effort to change; use thread_send to message another thread")
			}
			t, err := s.Store.Threads().Get(ctx, id)
			if err != nil {
				return nil, err
			}
			return map[string]any{"thread": threadSummary(ctx, m, t), "pending_settings": m.Pending(id)}, nil
		}
		t, applied, err := m.ConfigureThread(ctx, id, settings)
		if err != nil {
			return nil, err
		}
		out := map[string]any{"thread_id": id, "applied": applied}
		if applied == runtime.AppliedNow {
			out["provider"], out["model"], out["effort"] = t.Provider, t.Model, t.Effort
		} else {
			out["pending_settings"] = m.Pending(id)
		}
		if in.ContinueWith != "" {
			if err = m.Send(ctx, id, in.ContinueWith); err != nil {
				return nil, err
			}
			out["continued"] = true
		}
		if applied == runtime.AppliedAfterTurn && id == caller(ctx) {
			out["next"] = "End your turn now; the next one runs with the new selection."
		}
		return out, nil
	})
	add(srv, "thread_send", "Send a message to another thread, as the user would; it queues while that thread works.", func(ctx context.Context, in struct {
		ThreadID string `json:"thread_id"`
		Text     string `json:"text"`
	}) (any, error) {
		from := caller(ctx)
		if in.ThreadID == "" || in.ThreadID == from {
			return nil, apperr.New(apperr.Invalid, "thread_send needs another thread; to continue this one use thread_configure with continue_with")
		}
		if strings.TrimSpace(in.Text) == "" {
			return nil, apperr.New(apperr.Invalid, "thread_send needs text")
		}
		text := in.Text
		if from != "" {
			title := from
			if t, err := s.Store.Threads().Get(ctx, from); err == nil && t.Title != "" {
				title = t.Title
			}
			text = fmt.Sprintf("[Mensaje del hilo «%s» (%s)]\n\n%s", title, from, in.Text)
		}
		if err := m.SendFrom(ctx, in.ThreadID, from, text); err != nil {
			return nil, err
		}
		return map[string]any{"thread_id": in.ThreadID, "sent": true, "queued": len(m.Queue(in.ThreadID)) > 0}, nil
	})
	add(srv, "thread_create", "Create a new chat in the user's thread list (\"créame un hilo nuevo\"), here or in another project or environment. Not for subagents: use subagent_spawn.", func(ctx context.Context, in newThreadInput) (any, error) {
		var base data.Thread
		var origin *string
		if id := caller(ctx); id != "" {
			base, _ = s.Store.Threads().Get(ctx, id)
			origin = &id
		}
		req, err := threadInput(ctx, s, in, base.NodeID, base.ProjectID)
		if err != nil {
			return nil, err
		}
		req.Inherit, req.OriginID = &base, origin
		t, err := s.Threads.Create(ctx, req)
		if err != nil {
			return nil, err
		}
		return map[string]any{"thread_id": t.ID, "title": t.Title, "provider": t.Provider, "model": t.Model,
			"project_id": t.ProjectID, "environment": t.NodeID, "started": strings.TrimSpace(in.Prompt) != ""}, nil
	})
	add(srv, "list_environments", "List environments (this node and its peers) and whether they are connected.", func(ctx context.Context, _ struct{}) (any, error) {
		out := []map[string]any{{"id": s.Node.ID, "name": s.Node.Name, "connected": true, "this_node": true}}
		if peers, ok := s.Remote.(peerLister); ok {
			list, err := peers.Peers(ctx)
			if err != nil {
				return nil, err
			}
			for _, p := range list {
				if !p.Revoked {
					out = append(out, map[string]any{"id": p.ID, "name": p.Name, "connected": p.Connected})
				}
			}
		}
		return out, nil
	})
	add(srv, "subagent_spawn", "Launch a subagent (\"lanza un subagente…\", \"delega…\"): a hidden child thread whose result comes back here. Same folder, project, environment and model by default. Not for a new chat: use thread_create.", func(ctx context.Context, in newThreadInput) (any, error) {
		id := caller(ctx)
		if id == "" {
			return nil, apperr.New(apperr.Invalid, "subagent_spawn only works inside a Dory thread")
		}
		if strings.TrimSpace(in.Prompt) == "" {
			return nil, apperr.New(apperr.Invalid, "subagent_spawn needs a prompt")
		}
		parent, err := s.Store.Threads().Get(ctx, id)
		if err != nil {
			return nil, err
		}
		req, err := threadInput(ctx, s, in, parent.NodeID, parent.ProjectID)
		if err != nil {
			return nil, err
		}
		child, task, err := s.Threads.Spawn(ctx, id, req)
		if err != nil {
			return nil, err
		}
		return map[string]any{"task_id": task.ID, "thread_id": child.ID, "title": child.Title, "provider": child.Provider, "model": child.Model, "effort": child.Effort,
			"environment": child.NodeID, "status": task.Status, "next": "Its result will arrive here as a message when it finishes. Do not poll it; continue with other work or end your turn."}, nil
	})
	add(srv, "subagent_status", "Read a delegated task by task_id (status and result), or list this thread's tasks. Not to wait: the end wakes you.", func(ctx context.Context, in struct {
		TaskID string `json:"task_id,omitempty"`
	}) (any, error) {
		id := caller(ctx)
		if in.TaskID == "" {
			if id == "" {
				return nil, apperr.New(apperr.Invalid, "subagent_status needs task_id outside a Dory thread")
			}
			return m.Delegations(ctx, id)
		}
		d, err := m.Delegation(ctx, in.TaskID, true)
		if err != nil {
			return nil, err
		}
		out := map[string]any{"task": d}
		if child, err := s.Store.Threads().Get(ctx, d.ChildThreadID); err == nil {
			out["child"] = threadSummary(ctx, m, child)
		}
		return out, nil
	})
	add(srv, "subagent_cancel", "Cancel a running delegated task and stop its child.", func(ctx context.Context, in struct {
		TaskID string `json:"task_id"`
	}) (any, error) {
		d, err := m.CancelDelegation(ctx, in.TaskID)
		if err != nil {
			return nil, err
		}
		if d.Status == "cancelled" {
			if d.ChildNodeID == s.Node.ID {
				err = m.Interrupt(ctx, d.ChildThreadID)
			} else {
				err = s.PeerClient.InterruptThread(ctx, d.ChildNodeID, d.ChildThreadID)
			}
		}
		return map[string]any{"task_id": d.ID, "status": d.Status}, err
	})
	add(srv, "thread_stop", "Stop a thread's running turn and drop its queue.", func(ctx context.Context, in struct {
		ThreadID string `json:"thread_id"`
	}) (any, error) {
		if in.ThreadID == "" {
			return nil, apperr.New(apperr.Invalid, "thread_stop needs thread_id")
		}
		if err := m.Interrupt(ctx, in.ThreadID); err != nil {
			return nil, err
		}
		return map[string]any{"thread_id": in.ThreadID, "stopped": true}, nil
	})
}

func threadSummary(ctx context.Context, m *runtime.Manager, t data.Thread) map[string]any {
	out := map[string]any{"id": t.ID, "title": t.Title, "provider": t.Provider, "model": t.Model, "effort": t.Effort,
		"status": t.Status, "parent_id": t.ParentID, "task_id": t.TaskID, "project_id": t.ProjectID, "node_id": t.NodeID,
		"last_activity_at": t.LastActivityAt, "unread": t.Unread, "state_stale": t.StateStale, "archived": t.ArchivedAt != nil}
	if reqs, err := m.Requests(ctx, t.ID); err == nil {
		out["pending_requests"] = len(reqs)
	}
	return out
}

func activity(t data.Thread) string {
	if t.LastActivityAt != nil {
		return *t.LastActivityAt
	}
	return t.CreatedAt
}

type peerLister interface {
	Peers(context.Context) ([]mesh.Peer, error)
}

// findEnvironment resolves a node id or name; empty is this node.
func findEnvironment(ctx context.Context, s *api.Server, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "local" || name == s.Node.ID || strings.EqualFold(name, s.Node.Name) {
		return s.Node.ID, nil
	}
	if peers, ok := s.Remote.(peerLister); ok {
		list, err := peers.Peers(ctx)
		if err != nil {
			return "", err
		}
		for _, p := range list {
			if !p.Revoked && (p.ID == name || strings.EqualFold(p.Name, name)) {
				if !p.Connected {
					return "", apperr.Newf(apperr.NodeOffline, "the environment %s is not connected", p.Name)
				}
				return p.ID, nil
			}
		}
	}
	return "", apperr.Newf(apperr.NotFound, "unknown environment %q; use list_environments", name)
}

// findProjectByName resolves a project id, slug or name.
func findProjectByName(ctx context.Context, store *data.Store, name string) (data.Project, error) {
	projects, err := store.Projects().List(ctx, data.Filter{Limit: 1000})
	if err != nil {
		return data.Project{}, err
	}
	for _, p := range projects {
		if p.ID == name || strings.EqualFold(p.Slug, name) || strings.EqualFold(p.Name, name) {
			return p, nil
		}
	}
	return data.Project{}, apperr.Newf(apperr.NotFound, "unknown project %q; use list_projects", name)
}

type newThreadInput struct {
	Prompt      string `json:"prompt,omitempty" jsonschema:"First message; a subagent needs it and gets nothing else from this conversation"`
	Title       string `json:"title,omitempty"`
	Provider    string `json:"provider,omitempty" jsonschema:"Harness id from list_models; another one than this thread's needs model"`
	Model       string `json:"model,omitempty"`
	Effort      string `json:"effort,omitempty"`
	Project     string `json:"project,omitempty" jsonschema:"Project id, slug or name; this thread's by default"`
	Environment string `json:"environment,omitempty" jsonschema:"Node id or name to run on (list_environments); this thread's by default"`
	TaskID      string `json:"task_id,omitempty" jsonschema:"Task to link the new thread to"`
}

// threadInput resolves the names accepted by MCP before calling the application.
func threadInput(ctx context.Context, s *api.Server, in newThreadInput, node string, project *string) (app.CreateThread, error) {
	if in.Environment != "" {
		node = in.Environment
	}
	node, err := findEnvironment(ctx, s, node)
	if err != nil {
		return app.CreateThread{}, err
	}
	if in.Project != "" {
		p, err := findProjectByName(ctx, s.Store, in.Project)
		if err != nil {
			return app.CreateThread{}, err
		}
		project = &p.ID
	}
	req := app.CreateThread{Node: node, CreateThreadRequest: peerapi.CreateThreadRequest{
		ProjectID: project, Provider: in.Provider, Model: in.Model, Effort: in.Effort, Title: in.Title, Text: in.Prompt,
	}}
	if in.TaskID != "" {
		req.TaskID = &in.TaskID
	}
	return req, nil
}

func clip(text string) string {
	if r := []rune(text); len(r) > resultChars {
		return string(r[:resultChars]) + "…"
	}
	return text
}
