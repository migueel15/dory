package t3

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

// ViewMessage is one message of the thread view.
type ViewMessage struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Text      string `json:"text"`
	Streaming bool   `json:"streaming"`
	CreatedAt string `json:"created_at"`
}

// QueuedRun is a message waiting in T3's native queue for the active turn to end.
type QueuedRun struct {
	RunID    string `json:"run_id"`
	Text     string `json:"text"`
	Position int    `json:"position"`
	Held     bool   `json:"held"`
}

// PullRequestLink is a pull request T3 linked to the thread.
type PullRequestLink struct {
	Repository string `json:"repository"`
	Number     int    `json:"number"`
	URL        string `json:"url"`
}

// ThreadView is the normalized thread the chat view shows.
type ThreadView struct {
	ID              string            `json:"id"`
	Title           string            `json:"title"`
	ProviderID      string            `json:"provider_id"`
	Model           string            `json:"model"`
	Effort          string            `json:"effort"`
	EffortOption    string            `json:"effort_option"`
	InteractionMode string            `json:"interaction_mode"`
	RuntimeMode     string            `json:"runtime_mode"`
	Branch          string            `json:"branch"`
	WorktreePath    string            `json:"worktree_path"`
	ActiveRunID     string            `json:"active_run_id"`
	ActiveStatus    string            `json:"active_status"`
	PullRequests    []PullRequestLink `json:"pull_requests"`
	Messages        []ViewMessage     `json:"messages"`
	Queue           []QueuedRun       `json:"queue"`
	Pending         *Pending          `json:"pending"`
}

type wireOption struct {
	ID    string          `json:"id"`
	Value json.RawMessage `json:"value"`
}

type wireViewProjection struct {
	wireProjection
	Thread struct {
		Title          string `json:"title"`
		ModelSelection struct {
			InstanceID string       `json:"instanceId"`
			Model      string       `json:"model"`
			Options    []wireOption `json:"options"`
		} `json:"modelSelection"`
		InteractionMode   string            `json:"interactionMode"`
		RuntimeMode       string            `json:"runtimeMode"`
		Branch            *string           `json:"branch"`
		WorktreePath      *string           `json:"worktreePath"`
		LinkedPullRequest *PullRequestLink  `json:"linkedPullRequest"`
		BranchPullRequest *PullRequestLink  `json:"branchPullRequest"`
		PullRequests      []json.RawMessage `json:"pullRequests"`
	} `json:"thread"`
	Runs []struct {
		ID            string `json:"id"`
		Status        string `json:"status"`
		QueuePosition *int   `json:"queuePosition"`
		QueueHeld     bool   `json:"queueHeld"`
		UserMessageID string `json:"userMessageId"`
	} `json:"runs"`
	Checkpoints []struct {
		AppRunOrdinal *int   `json:"appRunOrdinal"`
		Status        string `json:"status"`
	} `json:"checkpoints"`
	FullMessages []struct {
		ID        string `json:"id"`
		Role      string `json:"role"`
		Text      string `json:"text"`
		Streaming bool   `json:"streaming"`
		CreatedAt string `json:"createdAt"`
	} `json:"messages"`
}

// activeStatuses are the run states during which a new message must queue or steer.
var activeStatuses = map[string]bool{"preparing": true, "starting": true, "running": true, "waiting": true}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ThreadView reads a thread with its whole conversation, queue and settings.
func (c Client) ThreadView(ctx context.Context, threadID string) (ThreadView, error) {
	var p wireViewProjection
	if err := c.call(ctx, "orchestration.getThreadProjection", map[string]any{"threadId": threadID}, &p); err != nil {
		return ThreadView{}, err
	}
	return p.view(threadID), nil
}

func (p wireViewProjection) view(threadID string) ThreadView {
	t := p.Thread
	v := ThreadView{
		ID: threadID, Title: t.Title, ProviderID: t.ModelSelection.InstanceID, Model: t.ModelSelection.Model,
		InteractionMode: t.InteractionMode, RuntimeMode: t.RuntimeMode,
		Branch: str(t.Branch), WorktreePath: str(t.WorktreePath),
		PullRequests: []PullRequestLink{}, Messages: []ViewMessage{}, Queue: []QueuedRun{},
	}
	for _, id := range effortOptionIDs {
		for _, o := range t.ModelSelection.Options {
			var s string
			if v.EffortOption == "" && o.ID == id && json.Unmarshal(o.Value, &s) == nil && s != "" {
				v.EffortOption, v.Effort = id, s
			}
		}
	}
	seen := map[string]bool{}
	addPR := func(pr PullRequestLink) {
		if pr.URL != "" && !seen[pr.URL] {
			seen[pr.URL] = true
			v.PullRequests = append(v.PullRequests, pr)
		}
	}
	for _, pr := range []*PullRequestLink{t.LinkedPullRequest, t.BranchPullRequest} {
		if pr != nil {
			addPR(*pr)
		}
	}
	for _, raw := range t.PullRequests {
		// Links either embed the pull request or carry its fields directly.
		var link struct {
			PullRequestLink
			PullRequest *PullRequestLink `json:"pullRequest"`
		}
		if json.Unmarshal(raw, &link) == nil {
			if link.PullRequest != nil {
				addPR(*link.PullRequest)
			}
			addPR(link.PullRequestLink)
		}
	}
	text := map[string]string{}
	for _, m := range p.FullMessages {
		text[m.ID] = m.Text
	}
	queued := map[string]bool{}
	for _, r := range p.Runs {
		switch {
		case r.Status == "queued":
			pos := 0
			if r.QueuePosition != nil {
				pos = *r.QueuePosition
			}
			v.Queue = append(v.Queue, QueuedRun{RunID: r.ID, Text: text[r.UserMessageID], Position: pos, Held: r.QueueHeld})
			queued[r.UserMessageID] = true
		case activeStatuses[r.Status]:
			v.ActiveRunID, v.ActiveStatus = r.ID, r.Status
		}
	}
	sort.SliceStable(v.Queue, func(i, j int) bool { return v.Queue[i].Position < v.Queue[j].Position })
	for _, m := range p.FullMessages {
		// Queued messages show in the queue, not in the conversation.
		if !queued[m.ID] {
			v.Messages = append(v.Messages, ViewMessage{ID: m.ID, Role: m.Role, Text: m.Text, Streaming: m.Streaming, CreatedAt: m.CreatedAt})
		}
	}
	v.Pending = p.context(0).Pending
	return v
}

// Send modes of a message in the thread view.
const (
	SendAuto  = "auto"  // start now, or queue after the active turn
	SendSteer = "steer" // inject into the active turn
)

// SendInput is a message from the thread view. An empty Model keeps the
// thread's current model.
type SendInput struct {
	Text         string
	Mode         string
	ProviderID   string
	Model        string
	EffortOption string
	Effort       string
}

// Send dispatches a message with T3's native queue or steer. Steering without an
// active turn starts the message right away.
func (c Client) Send(ctx context.Context, threadID string, in SendInput) error {
	if strings.TrimSpace(in.Text) == "" {
		return errors.New("t3: empty message")
	}
	mode := map[string]any{"type": "queue_after_active"}
	if in.Mode == SendSteer {
		v, err := c.ThreadView(ctx, threadID)
		if err != nil {
			return err
		}
		if v.ActiveRunID != "" {
			mode = map[string]any{"type": "steer_active", "targetRunId": v.ActiveRunID}
		}
	}
	cmd := map[string]any{
		"type": "message.dispatch", "threadId": threadID, "messageId": newID(), "text": in.Text,
		"attachments": []any{}, "createdBy": "user", "creationSource": "web", "dispatchMode": mode,
	}
	if in.Model != "" {
		sel := map[string]any{"instanceId": in.ProviderID, "model": in.Model}
		if in.EffortOption != "" && in.Effort != "" {
			sel["options"] = []map[string]any{{"id": in.EffortOption, "value": in.Effort}}
		}
		cmd["modelSelection"] = sel
	}
	return c.Dispatch(ctx, cmd)
}

// EditQueued replaces the text of a queued message.
func (c Client) EditQueued(ctx context.Context, threadID, runID, text string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("t3: empty message")
	}
	return c.Dispatch(ctx, map[string]any{"type": "queued-run.edit", "threadId": threadID, "runId": runID, "text": text, "attachments": []any{}})
}

// ReorderQueued moves a queued message before another one, or last when beforeRunID is empty.
func (c Client) ReorderQueued(ctx context.Context, threadID, runID, beforeRunID string) error {
	var before any
	if beforeRunID != "" {
		before = beforeRunID
	}
	return c.Dispatch(ctx, map[string]any{"type": "queued-run.reorder", "threadId": threadID, "runId": runID, "beforeRunId": before})
}

// CancelQueued removes a message from the queue.
func (c Client) CancelQueued(ctx context.Context, threadID, runID string) error {
	return c.Dispatch(ctx, map[string]any{"type": "queued-run.cancel", "threadId": threadID, "runId": runID})
}

// SteerQueued injects a queued message into the active turn.
func (c Client) SteerQueued(ctx context.Context, threadID, runID, activeRunID string) error {
	return c.Dispatch(ctx, map[string]any{"type": "queued-message.promote-to-steer", "threadId": threadID, "queuedRunId": runID, "targetRunId": activeRunID})
}

// SetInteractionMode switches the thread between chat (default) and plan.
func (c Client) SetInteractionMode(ctx context.Context, threadID, mode string) error {
	if mode != "default" && mode != "plan" {
		return errors.New("t3: invalid interaction mode")
	}
	return c.Dispatch(ctx, map[string]any{"type": "thread.interaction-mode.set", "threadId": threadID, "interactionMode": mode})
}

// runtimeModes are the access levels T3 accepts.
var runtimeModes = map[string]bool{"approval-required": true, "auto-accept-edits": true, "auto": true, "full-access": true}

// SetRuntimeMode changes the thread's access level.
func (c Client) SetRuntimeMode(ctx context.Context, threadID, mode string) error {
	if !runtimeModes[mode] {
		return errors.New("t3: invalid runtime mode")
	}
	return c.Dispatch(ctx, map[string]any{"type": "thread.runtime-mode.set", "threadId": threadID, "runtimeMode": mode})
}

// ThreadDiff returns every change the thread made in its worktree, from T3's
// checkpoints up to the latest ready turn. Empty when no turn changed files.
func (c Client) ThreadDiff(ctx context.Context, threadID string) (string, error) {
	var p wireViewProjection
	if err := c.call(ctx, "orchestration.getThreadProjection", map[string]any{"threadId": threadID}, &p); err != nil {
		return "", err
	}
	turns := 0
	for _, cp := range p.Checkpoints {
		if cp.Status == "ready" && cp.AppRunOrdinal != nil {
			turns = max(turns, *cp.AppRunOrdinal)
		}
	}
	if turns == 0 {
		return "", nil
	}
	var out struct {
		Diff string `json:"diff"`
	}
	err := c.call(ctx, "orchestration.getFullThreadDiff", map[string]any{"threadId": threadID, "toTurnCount": turns, "ignoreWhitespace": false}, &out)
	return out.Diff, err
}
