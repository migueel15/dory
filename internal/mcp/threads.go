package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"

	"github.com/migueel15/dory/internal/api"
	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/mesh"
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
		return "", errors.New("thread_id is required outside a Dory thread")
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
	add(srv, "list_models", "List the harnesses (providers) of this node with their models and efforts. Use it to turn what the user says, like haiku, opus, sonnet or codex, into a provider and a model slug for thread_configure.", func(ctx context.Context, in struct {
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
	add(srv, "list_threads", "Find threads by project, task, parent, status or words of their title, newest activity first. Use it when the user names another thread.", func(ctx context.Context, in threadListInput) (any, error) {
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
	add(srv, "get_thread", "Read a thread (this conversation by default): its selection, status, queue, pending model change, children and last messages. last_result is its last answer.", func(ctx context.Context, in struct {
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
	add(srv, "thread_configure", "Read or change the provider, model and effort of a thread (this conversation by default). Use it when the user asks to switch model or harness, e.g. \"cámbiate a haiku\" or \"pasa el hilo X a codex\". Names are resolved against list_models; a provider switch needs a model and hands the history over. On a running thread, such as your own, the change applies when the turn ends; set continue_with to start the next turn with the new selection, e.g. to execute a plan after designing it. Permission modes never change.", func(ctx context.Context, in threadConfigureInput) (any, error) {
		id, err := target(ctx, in.ThreadID)
		if err != nil {
			return nil, err
		}
		settings := runtime.ThreadSettings{Provider: in.Provider, Model: in.Model, Effort: in.Effort}
		if in.Provider == nil && in.Model == nil && in.Effort == nil {
			if in.ContinueWith != "" {
				return nil, errors.New("continue_with needs a provider, model or effort to change; use thread_send to message another thread")
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
	add(srv, "thread_send", "Send a message to another thread, as the user would. It starts a turn there, or waits in its queue while the thread works. The message says which thread sent it.", func(ctx context.Context, in struct {
		ThreadID string `json:"thread_id"`
		Text     string `json:"text"`
	}) (any, error) {
		from := caller(ctx)
		if in.ThreadID == "" || in.ThreadID == from {
			return nil, errors.New("thread_send needs another thread; to continue this one use thread_configure with continue_with")
		}
		if strings.TrimSpace(in.Text) == "" {
			return nil, errors.New("thread_send needs text")
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
	add(srv, "thread_create", "Create a new, independent thread (a new chat in the user's list), e.g. when the user asks \"créame un hilo nuevo\" or \"abre un chat en el proyecto X en el entorno Y que…\". It is not a subagent: it does not report back here. project and environment take an id or a name (list_projects; environments are this node and its peers, see list_environments). By default it takes this thread's project, provider, model and permission mode and runs on this node; prompt, if given, starts its first turn. For work whose result should come back here, use thread_spawn instead.", func(ctx context.Context, in struct {
		Prompt      string `json:"prompt,omitempty" jsonschema:"First message of the new thread; empty leaves it waiting for the user"`
		Title       string `json:"title,omitempty"`
		Provider    string `json:"provider,omitempty" jsonschema:"Harness id from list_models; another one than this thread's needs model"`
		Model       string `json:"model,omitempty"`
		Effort      string `json:"effort,omitempty"`
		Project     string `json:"project,omitempty" jsonschema:"Project id, slug or name; this thread's by default; none for a scratch thread"`
		Environment string `json:"environment,omitempty" jsonschema:"Node id or name to run on; this node by default"`
		TaskID      string `json:"task_id,omitempty" jsonschema:"Task to link the new thread to"`
	}) (any, error) {
		var base data.Thread
		if id := caller(ctx); id != "" {
			base, _ = s.Store.Threads().Get(ctx, id)
		}
		node, err := findEnvironment(ctx, s, in.Environment)
		if err != nil {
			return nil, err
		}
		n := runtime.NewThread{Provider: base.Provider, Model: base.Model, Effort: base.Effort, RuntimeMode: base.RuntimeMode, ProjectID: base.ProjectID, Title: in.Title}
		if in.Project != "" {
			p, err := findProjectByName(ctx, s.Store, in.Project)
			if err != nil {
				return nil, err
			}
			n.ProjectID = &p.ID
		}
		if in.TaskID != "" {
			n.TaskID = &in.TaskID
		}
		if in.Provider != "" && !strings.EqualFold(in.Provider, n.Provider) {
			if in.Model == "" {
				return nil, fmt.Errorf("choose a model for provider %s; use list_models", in.Provider)
			}
			n.Provider, n.Model, n.Effort = strings.ToLower(in.Provider), "", ""
		}
		if n.Provider == "" {
			return nil, errors.New("thread_create needs a provider outside a Dory thread; use list_models")
		}
		if in.Model != "" {
			n.Model = in.Model
		}
		if in.Effort != "" {
			n.Effort = in.Effort
		}
		if n.Title == "" {
			n.Title = firstLine(in.Prompt, 60)
		}
		if n.Title == "" {
			n.Title = "Hilo nuevo"
		}
		if node != s.Node.ID {
			// The peer checks the selection against its own harnesses.
			id, err := createOnPeer(ctx, s, node, n, in.Prompt)
			if err != nil {
				return nil, err
			}
			return map[string]any{"thread_id": id, "title": n.Title, "provider": n.Provider, "model": n.Model, "project_id": n.ProjectID, "environment": node, "started": in.Prompt != ""}, nil
		}
		settings := runtime.ThreadSettings{Model: &n.Model}
		if n.Effort != "" {
			settings.Effort = &n.Effort
		}
		if n.Model == "" {
			settings.Model = nil
		}
		if err := m.ResolveSettings(ctx, data.Thread{Provider: n.Provider, Model: n.Model}, &settings); err != nil {
			return nil, err
		}
		if settings.Model != nil {
			n.Model = *settings.Model
		}
		if settings.Effort != nil {
			n.Effort = *settings.Effort
		}
		if n.ProjectID != nil {
			if n.RepoPath, err = s.ProjectDir(ctx, *n.ProjectID); err != nil {
				return nil, err
			}
		}
		t, err := m.Create(ctx, n)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(in.Prompt) != "" {
			if err = m.Send(ctx, t.ID, in.Prompt); err != nil {
				return nil, err
			}
		}
		return map[string]any{"thread_id": t.ID, "title": t.Title, "provider": t.Provider, "model": t.Model, "project_id": t.ProjectID, "environment": s.Node.ID, "started": in.Prompt != ""}, nil
	})
	add(srv, "list_environments", "List the environments a thread can run on: this node and its paired peers, with whether they are connected.", func(ctx context.Context, _ struct{}) (any, error) {
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
	add(srv, "thread_spawn", "Launch a subagent: a hidden child thread of this conversation that works on prompt in the same folder and task, with its own provider and model (list_models), e.g. when the user asks for a subagent (\"lanza un subagente con haiku que revise X\") or to split work in parallel. It does not show in the thread list; for a new chat the user will see there, use thread_create. The child does not see this conversation, so put every detail it needs in prompt. Its result arrives here as a message when its turn ends: do not poll it; continue with other work or end your turn. Subagents cannot launch subagents, and at most 5 work at once.", func(ctx context.Context, in struct {
		Prompt   string `json:"prompt" jsonschema:"Self-contained task for the subagent"`
		Title    string `json:"title,omitempty" jsonschema:"Short title; the first line of prompt by default"`
		Provider string `json:"provider,omitempty" jsonschema:"Harness id from list_models; this thread's by default; another one needs model"`
		Model    string `json:"model,omitempty" jsonschema:"Model slug or name; this thread's by default"`
		Effort   string `json:"effort,omitempty"`
	}) (any, error) {
		parent := caller(ctx)
		if parent == "" {
			return nil, errors.New("thread_spawn only works inside a Dory thread")
		}
		child, err := m.Spawn(ctx, parent, runtime.Subagent{Prompt: in.Prompt, Title: in.Title, Provider: in.Provider, Model: in.Model, Effort: in.Effort})
		if err != nil {
			return nil, err
		}
		return map[string]any{"thread_id": child.ID, "title": child.Title, "provider": child.Provider, "model": child.Model, "effort": child.Effort,
			"status": child.Status, "next": "Its result will arrive here as a message when it finishes. Do not poll it; continue with other work or end your turn."}, nil
	})
	add(srv, "thread_stop", "Stop the running turn of a thread and drop its queued messages.", func(ctx context.Context, in struct {
		ThreadID string `json:"thread_id"`
	}) (any, error) {
		if in.ThreadID == "" {
			return nil, errors.New("thread_stop needs thread_id")
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
		"last_activity_at": t.LastActivityAt, "archived": t.ArchivedAt != nil}
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
					return "", fmt.Errorf("the environment %s is not connected", p.Name)
				}
				return p.ID, nil
			}
		}
	}
	return "", fmt.Errorf("unknown environment %q; use list_environments", name)
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
	return data.Project{}, fmt.Errorf("unknown project %q; use list_projects", name)
}

// createOnPeer asks a peer to create and start a thread, through the mesh.
func createOnPeer(ctx context.Context, s *api.Server, node string, n runtime.NewThread, prompt string) (string, error) {
	body, _ := json.Marshal(map[string]any{"project_id": n.ProjectID, "task_id": n.TaskID, "provider": n.Provider, "model": n.Model,
		"effort": n.Effort, "runtime_mode": n.RuntimeMode, "title": n.Title, "text": prompt})
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/threads", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	if s.Remote == nil || !s.Remote.Forward(rec, req, node) {
		return "", errors.New("the environment is not reachable")
	}
	var out struct {
		ThreadID string `json:"thread_id"`
		Message  string `json:"message"`
		Error    any    `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code >= 300 || out.ThreadID == "" {
		return "", fmt.Errorf("the environment refused the thread (%d): %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	return out.ThreadID, nil
}

func firstLine(text string, limit int) string {
	text = strings.TrimSpace(text)
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	if r := []rune(text); len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return text
}

func clip(text string) string {
	if r := []rune(text); len(r) > resultChars {
		return string(r[:resultChars]) + "…"
	}
	return text
}
