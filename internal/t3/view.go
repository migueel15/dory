package t3

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
)

// ViewMessage is one entry of the thread view: a user or assistant message,
// the model's reasoning (role "reasoning"), a tool it ran (role "tool", with
// Title, Detail and Status), a proposed plan in markdown (role "plan") or the
// point where the user stopped a turn (role "interrupt").
type ViewMessage struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Text      string `json:"text"`
	Streaming bool   `json:"streaming"`
	CreatedAt string `json:"created_at"`
	Title     string `json:"title,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Status    string `json:"status,omitempty"`
	// From and To are the models of a provider handoff (role "handoff"),
	// with their provider instances in FromProvider and ToProvider.
	From         string `json:"from,omitempty"`
	To           string `json:"to,omitempty"`
	FromProvider string `json:"from_provider,omitempty"`
	ToProvider   string `json:"to_provider,omitempty"`
	// HTML is the page of an html_render call (role "html").
	HTML *HTMLRender `json:"html,omitempty"`
	// Mentions are the @paths a user message referenced.
	Mentions []string `json:"mentions,omitempty"`
	// Skills are the $skills a user message invoked.
	Skills []string `json:"skills,omitempty"`
	// Attachments are the files sent with a user message.
	Attachments []Attachment `json:"attachments,omitempty"`
}

// wireAttachment is a chat attachment as T3 stores it on a message.
type wireAttachment struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	MimeType  string `json:"mimeType"`
	SizeBytes int    `json:"sizeBytes"`
}

func attachmentsOf(in []wireAttachment) []Attachment {
	var out []Attachment
	for _, a := range in {
		out = append(out, Attachment{Type: a.Type, ID: a.ID, Name: a.Name, MimeType: a.MimeType, SizeBytes: a.SizeBytes})
	}
	return out
}

// wireTurnItem is one step of a turn: messages, reasoning and tool calls.
type wireTurnItem struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Status    string          `json:"status"`
	Streaming bool            `json:"streaming"`
	Title     *string         `json:"title"`
	ToolName  *string         `json:"toolName"`
	Text      *string         `json:"text"`
	Input     json.RawMessage `json:"input"`
	MessageID *string         `json:"messageId"`
	Ordinal   int             `json:"ordinal"`
	StartedAt string          `json:"startedAt"`
	RunID     string          `json:"runId"`
	Markdown  *string         `json:"markdown"`
	// A provider handoff names the models before and after it.
	FromModelSelections []struct {
		InstanceID string `json:"instanceId"`
		Model      string `json:"model"`
	} `json:"fromModelSelections"`
	FromProviderInstanceIDs []string        `json:"fromProviderInstanceIds"`
	ToProviderInstanceID    *string         `json:"toProviderInstanceId"`
	ToModel                 *string         `json:"toModel"`
	Output                  json.RawMessage `json:"output"`
	// A secret request (request_secret) asks the user for one value.
	Label        *string `json:"label"`
	Reason       *string `json:"reason"`
	Placeholder  *string `json:"placeholder"`
	Delivery     *string `json:"delivery"`
	SecretStatus *string `json:"secretStatus"`
}

// HTMLRender is a page the agent showed with T3's html_render tool.
type HTMLRender struct {
	AttachmentID string  `json:"attachment_id"`
	Title        string  `json:"title"`
	Height       float64 `json:"height"`
}

// htmlRenderOf finds the page an html_render call stored, wherever the tool
// result nests it (an object, or JSON inside the text content).
func htmlRenderOf(it wireTurnItem) *HTMLRender {
	name := str(it.ToolName)
	if name == "" {
		name = str(it.Title)
	}
	if !strings.HasSuffix(name, "html_render") || len(it.Output) == 0 {
		return nil
	}
	var walk func(v any, depth int) *HTMLRender
	walk = func(v any, depth int) *HTMLRender {
		if depth > 8 {
			return nil
		}
		switch x := v.(type) {
		case string:
			var inner any
			if strings.Contains(x, "htmlRender") && json.Unmarshal([]byte(x), &inner) == nil {
				return walk(inner, depth+1)
			}
		case []any:
			for _, e := range x {
				if h := walk(e, depth+1); h != nil {
					return h
				}
			}
		case map[string]any:
			if hr, ok := x["htmlRender"].(map[string]any); ok {
				if id, _ := hr["attachmentId"].(string); id != "" {
					h := &HTMLRender{AttachmentID: id}
					h.Title, _ = hr["title"].(string)
					h.Height, _ = hr["height"].(float64)
					return h
				}
			}
			for _, e := range x {
				if h := walk(e, depth+1); h != nil {
					return h
				}
			}
		}
		return nil
	}
	var v any
	if json.Unmarshal(it.Output, &v) != nil {
		return nil
	}
	return walk(v, 0)
}

// hiddenItems are turn items the chat does not show as steps.
var hiddenItems = map[string]bool{"checkpoint": true, "user_input_request": true, "user_message": true, "assistant_message": true, "run_interrupt_request": true}

// toolDetail summarizes a tool input: a command as is, anything else as compact JSON.
func toolDetail(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	d := string(raw)
	if len(d) > 400 {
		d = d[:400] + "…"
	}
	return d
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
	ID              string `json:"id"`
	Title           string `json:"title"`
	ProviderID      string `json:"provider_id"`
	Model           string `json:"model"`
	Effort          string `json:"effort"`
	EffortOption    string `json:"effort_option"`
	InteractionMode string `json:"interaction_mode"`
	RuntimeMode     string `json:"runtime_mode"`
	Branch          string `json:"branch"`
	// ProjectID is the T3 project the thread belongs to.
	ProjectID    string `json:"project_id"`
	WorktreePath string `json:"worktree_path"`
	ActiveRunID  string `json:"active_run_id"`
	ActiveStatus string `json:"active_status"`
	// Interrupted is true when the last run was stopped and nothing runs now.
	Interrupted  bool              `json:"interrupted"`
	PullRequests []PullRequestLink `json:"pull_requests"`
	Messages     []ViewMessage     `json:"messages"`
	Queue        []QueuedRun       `json:"queue"`
	Pending      *Pending          `json:"pending"`
}

type wireOption struct {
	ID    string          `json:"id"`
	Value json.RawMessage `json:"value"`
}

type wireViewProjection struct {
	wireProjection
	Thread struct {
		Title          string `json:"title"`
		ProjectID      string `json:"projectId"`
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
		ID          string           `json:"id"`
		Role        string           `json:"role"`
		Text        string           `json:"text"`
		Streaming   bool             `json:"streaming"`
		CreatedAt   string           `json:"createdAt"`
		Attachments []wireAttachment `json:"attachments"`
		Context     *struct {
			Records []struct {
				Kind string `json:"kind"`
				Path string `json:"path"`
				Name string `json:"name"`
			} `json:"records"`
		} `json:"context"`
	} `json:"messages"`
	TurnItems []wireTurnItem `json:"turnItems"`
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
	var raw json.RawMessage
	if err := c.call(ctx, "orchestration.getThreadProjection", map[string]any{"threadId": threadID}, &raw); err != nil {
		return ThreadView{}, err
	}
	var p wireViewProjection
	if err := json.Unmarshal(raw, &p); err != nil {
		return ThreadView{}, err
	}
	// The view's own TurnItems shadows the embedded projection's, which the
	// pending request needs (questions and options), so decode it on its own.
	if err := json.Unmarshal(raw, &p.wireProjection); err != nil {
		return ThreadView{}, err
	}
	return p.view(threadID), nil
}

func (p wireViewProjection) view(threadID string) ThreadView {
	t := p.Thread
	v := ThreadView{
		ID: threadID, Title: t.Title, ProviderID: t.ModelSelection.InstanceID, Model: t.ModelSelection.Model,
		InteractionMode: t.InteractionMode, RuntimeMode: t.RuntimeMode,
		Branch: str(t.Branch), WorktreePath: str(t.WorktreePath), ProjectID: t.ProjectID,
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
	for i := len(p.Runs) - 1; i >= 0; i-- {
		if r := p.Runs[i]; r.Status != "queued" {
			v.Interrupted = v.ActiveRunID == "" && (r.Status == "interrupted" || r.Status == "cancelled")
			break
		}
	}
	sort.SliceStable(v.Queue, func(i, j int) bool { return v.Queue[i].Position < v.Queue[j].Position })
	msgs := map[string]ViewMessage{}
	for _, m := range p.FullMessages {
		// Queued messages show in the queue, not in the conversation.
		if !queued[m.ID] {
			msgs[m.ID] = ViewMessage{ID: m.ID, Role: m.Role, Text: m.Text, Streaming: m.Streaming, CreatedAt: m.CreatedAt, Attachments: attachmentsOf(m.Attachments)}
			if m.Context != nil {
				for _, rec := range m.Context.Records {
					vm := msgs[m.ID]
					switch {
					case rec.Kind == "mention" && rec.Path != "":
						vm.Mentions = append(vm.Mentions, rec.Path)
					case rec.Kind == "skill" && rec.Name != "":
						vm.Skills = append(vm.Skills, rec.Name)
					}
					msgs[m.ID] = vm
				}
			}
		}
	}
	runStatus := map[string]string{}
	for _, r := range p.Runs {
		runStatus[r.ID] = r.Status
	}
	shown := map[string]bool{}
	v.Messages = append(v.Messages, timeline(p.TurnItems, msgs, runStatus, shown, false)...)
	for _, m := range p.FullMessages {
		if mm, ok := msgs[m.ID]; ok && !shown[m.ID] {
			v.Messages = append(v.Messages, mm)
		}
	}
	v.Pending = p.context(0).Pending
	return v
}

// timeline turns turn items into view entries in order: messages, reasoning,
// tools, plans and stops. Messages come from msgs; with fromItems, a message
// missing there is rebuilt from the item's own text (older history pages).
func timeline(in []wireTurnItem, msgs map[string]ViewMessage, runStatus map[string]string, shown map[string]bool, fromItems bool) []ViewMessage {
	out := []ViewMessage{}
	items := append([]wireTurnItem(nil), in...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].Ordinal < items[j].Ordinal })
	for _, it := range items {
		// Steps left open by a run that already ended did not keep running.
		if st := runStatus[it.RunID]; it.RunID != "" && st != "" && !activeStatuses[st] && st != "queued" && (it.Streaming || it.Status == "running" || it.Status == "inProgress") {
			it.Streaming, it.Status = false, st
		}
		// Older pages hold only finished turns.
		if fromItems && (it.Streaming || it.Status == "running" || it.Status == "inProgress") {
			it.Streaming, it.Status = false, "completed"
		}
		switch {
		case it.MessageID != nil:
			m, ok := msgs[*it.MessageID]
			if !ok && fromItems && (it.Type == "user_message" || it.Type == "assistant_message") {
				m, ok = ViewMessage{ID: *it.MessageID, Role: strings.TrimSuffix(it.Type, "_message"), Text: str(it.Text), CreatedAt: it.StartedAt}, true
			}
			if ok && !shown[m.ID] {
				shown[m.ID] = true
				out = append(out, m)
			}
		case it.Type == "proposed_plan":
			// A plan the model proposed shows as its own card, not as a step.
			out = append(out, ViewMessage{ID: it.ID, Role: "plan", Text: str(it.Markdown), Streaming: it.Streaming, CreatedAt: it.StartedAt, Status: it.Status})
		case it.Type == "run_interrupt_result":
			// A stopped turn shows as a divider, not as a step.
			out = append(out, ViewMessage{ID: it.ID, Role: "interrupt", Text: "Parado por el usuario", CreatedAt: it.StartedAt})
		case it.Type == "secret_request":
			// The agent asks for a secret: a card that sends it to T3 without the chat
			// ever holding it. Title is the label, Text the reason, Detail the placeholder.
			out = append(out, ViewMessage{ID: it.ID, Role: "secret", Title: str(it.Label), Text: str(it.Reason), Detail: str(it.Placeholder), Status: str(it.SecretStatus), CreatedAt: it.StartedAt})
		case htmlRenderOf(it) != nil:
			// A page the agent rendered shows inline, like T3 Code does.
			out = append(out, ViewMessage{ID: it.ID, Role: "html", CreatedAt: it.StartedAt, HTML: htmlRenderOf(it)})
		case it.Type == "handoff":
			// The thread changed model: a divider from the old one to the new one.
			h := ViewMessage{ID: it.ID, Role: "handoff", CreatedAt: it.StartedAt, To: str(it.ToModel), ToProvider: str(it.ToProviderInstanceID)}
			var from []string
			for _, m := range it.FromModelSelections {
				if m.Model != "" && m.Model != h.To {
					from = append(from, m.Model)
				}
				if h.FromProvider == "" {
					h.FromProvider = m.InstanceID
				}
			}
			if h.FromProvider == "" && len(it.FromProviderInstanceIDs) > 0 {
				h.FromProvider = it.FromProviderInstanceIDs[0]
			}
			h.From = strings.Join(from, ", ")
			out = append(out, h)
		case it.Type == "reasoning":
			out = append(out, ViewMessage{ID: it.ID, Role: "reasoning", Text: str(it.Text), Streaming: it.Streaming, CreatedAt: it.StartedAt, Status: it.Status})
		case !hiddenItems[it.Type]:
			title := str(it.Title)
			if title == "" {
				title = str(it.ToolName)
			}
			if title == "" {
				title = strings.ReplaceAll(it.Type, "_", " ")
			}
			out = append(out, ViewMessage{ID: it.ID, Role: "tool", Title: title, Detail: toolDetail(it.Input), CreatedAt: it.StartedAt, Status: it.Status})
		}
	}
	return out
}

// HistoryPage is an older stretch of a thread, oldest first.
type HistoryPage struct {
	Messages   []ViewMessage `json:"messages"`
	NextCursor string        `json:"next_cursor"`
	HasMore    bool          `json:"has_more"`
}

// HistoryCursor returns where the thread's older history starts, or "" when
// the view already holds all of it.
func (c Client) HistoryCursor(ctx context.Context, threadID string) (string, error) {
	var b struct {
		HistoryCursor  *string `json:"historyCursor"`
		HasMoreHistory bool    `json:"hasMoreHistory"`
	}
	if err := c.get(ctx, "/api/orchestration/threads/"+url.PathEscape(threadID)+"/bounded", true, &b); err != nil {
		return "", err
	}
	if !b.HasMoreHistory || b.HistoryCursor == nil {
		return "", nil
	}
	return *b.HistoryCursor, nil
}

// History reads one older page of a thread from T3's paged history.
func (c Client) History(ctx context.Context, threadID, cursor string) (HistoryPage, error) {
	var h struct {
		Items []struct {
			Item wireTurnItem `json:"item"`
		} `json:"items"`
		NextCursor     *string `json:"nextCursor"`
		HasMoreHistory bool    `json:"hasMoreHistory"`
	}
	path := "/api/orchestration/threads/" + url.PathEscape(threadID) + "/history?cursor=" + url.QueryEscape(cursor)
	if err := c.get(ctx, path, true, &h); err != nil {
		return HistoryPage{}, err
	}
	items := make([]wireTurnItem, 0, len(h.Items))
	for _, it := range h.Items {
		items = append(items, it.Item)
	}
	p := HistoryPage{Messages: timeline(items, map[string]ViewMessage{}, map[string]string{}, map[string]bool{}, true), HasMore: h.HasMoreHistory}
	if h.NextCursor != nil && h.HasMoreHistory {
		p.NextCursor = *h.NextCursor
	}
	return p, nil
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
	Attachments  []Attachment
	// Skills and Mentions are the $skills and @paths picked in the composer;
	// they travel as T3 context records, as T3 Code's composer sends them.
	Skills   []string
	Mentions []string
}

// contextRecords builds T3's message context from picked skills and paths.
func contextRecords(skills, mentions []string) map[string]any {
	records := []any{}
	seen := map[string]bool{}
	for _, name := range skills {
		if name = strings.TrimSpace(name); name != "" && !seen["s:"+name] {
			seen["s:"+name] = true
			records = append(records, map[string]any{"version": 1, "contextId": newID(), "label": name, "kind": "skill", "name": name})
		}
	}
	for _, path := range mentions {
		if path = strings.TrimSpace(path); path != "" && !seen["m:"+path] {
			seen["m:"+path] = true
			records = append(records, map[string]any{"version": 1, "contextId": newID(), "label": path, "kind": "mention", "path": path})
		}
	}
	if len(records) == 0 {
		return nil
	}
	return map[string]any{"version": 1, "records": records}
}

// Send dispatches a message with T3's native queue or steer. Steering without an
// active turn starts the message right away.
func (c Client) Send(ctx context.Context, threadID string, in SendInput) error {
	if strings.TrimSpace(in.Text) == "" && len(in.Attachments) == 0 {
		return errors.New("t3: empty message")
	}
	attachments := []any{}
	for _, a := range in.Attachments {
		attachments = append(attachments, a.wire())
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
		"attachments": attachments, "createdBy": "user", "creationSource": "web", "dispatchMode": mode,
	}
	if ctxRecords := contextRecords(in.Skills, in.Mentions); ctxRecords != nil {
		cmd["context"] = ctxRecords
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

// Interrupt stops the active turn and holds the queue, as T3's stop button does.
func (c Client) Interrupt(ctx context.Context, threadID, runID string) error {
	return c.Dispatch(ctx, map[string]any{"type": "run.interrupt", "threadId": threadID, "runId": runID, "holdQueue": true})
}

// ResumeQueue releases a held queue so its next message starts.
func (c Client) ResumeQueue(ctx context.Context, threadID string) error {
	return c.Dispatch(ctx, map[string]any{"type": "queue.resume", "threadId": threadID})
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

// AnswerSecret answers a request_secret card: with a value T3 stores it and
// hands the agent a one-use reference; with decline the agent goes on without it.
func (c Client) AnswerSecret(ctx context.Context, threadID, itemID, secret string, decline bool) error {
	answer := map[string]any{"type": "decline"}
	if !decline {
		if strings.TrimSpace(secret) == "" {
			return errors.New("t3: empty secret")
		}
		answer = map[string]any{"type": "save", "secret": strings.TrimSpace(secret)}
	}
	return c.call(ctx, "secrets.answerRequest", map[string]any{"threadId": threadID, "turnItemId": itemID, "answer": answer}, nil)
}

// ProjectEntry is a file or folder of the thread's workspace, for @ mentions.
type ProjectEntry struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

// SearchEntries finds files and folders under cwd whose path matches query.
func (c Client) SearchEntries(ctx context.Context, cwd, query string) ([]ProjectEntry, error) {
	var out struct {
		Entries []ProjectEntry `json:"entries"`
	}
	err := c.call(ctx, "projects.searchEntries", map[string]any{"cwd": cwd, "query": query, "limit": 30}, &out)
	return out.Entries, err
}

// ThreadWorktree returns a thread's project root and its worktree ("" when it
// works in the root).
func (c Client) ThreadWorktree(ctx context.Context, threadID string) (root, worktree string, err error) {
	var p struct {
		Thread struct {
			WorktreePath *string `json:"worktreePath"`
			ProjectID    string  `json:"projectId"`
		} `json:"thread"`
	}
	if err := c.call(ctx, "orchestration.getThreadProjection", map[string]any{"threadId": threadID}, &p); err != nil {
		return "", "", err
	}
	root, err = c.ProjectRoot(ctx, p.Thread.ProjectID)
	return root, str(p.Thread.WorktreePath), err
}

// ProjectRoot is the folder of a T3 project.
func (c Client) ProjectRoot(ctx context.Context, projectID string) (string, error) {
	cat, err := c.Catalog(ctx)
	if err != nil {
		return "", err
	}
	for _, pr := range cat.Projects {
		if pr.ID == projectID && pr.Root != "" {
			return pr.Root, nil
		}
	}
	return "", errors.New("t3: project folder not found")
}

// ThreadCwd is the folder a thread works in: its worktree, else its project's root.
func (c Client) ThreadCwd(ctx context.Context, threadID string) (string, error) {
	var p struct {
		Thread struct {
			WorktreePath *string `json:"worktreePath"`
			ProjectID    string  `json:"projectId"`
		} `json:"thread"`
	}
	if err := c.call(ctx, "orchestration.getThreadProjection", map[string]any{"threadId": threadID}, &p); err != nil {
		return "", err
	}
	if w := str(p.Thread.WorktreePath); w != "" {
		return w, nil
	}
	cat, err := c.Catalog(ctx)
	if err != nil {
		return "", err
	}
	for _, pr := range cat.Projects {
		if pr.ID == p.Thread.ProjectID {
			return pr.Root, nil
		}
	}
	return "", errors.New("t3: thread folder not found")
}
