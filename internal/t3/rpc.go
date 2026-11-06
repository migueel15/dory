package t3

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// rpcTimeout bounds one write call, including the WebSocket handshake.
const rpcTimeout = 15 * time.Second

// rpcReadLimit admits the projection of a long thread.
const rpcReadLimit = 64 << 20

// Provider approval decisions accepted by T3.
const (
	DecisionAccept  = "accept"
	DecisionDecline = "decline"
)

type rpcMessage struct {
	Tag       string          `json:"_tag"`
	RequestID string          `json:"requestId"`
	Exit      json.RawMessage `json:"exit"`
}

type rpcExit struct {
	Tag   string          `json:"_tag"`
	Value json.RawMessage `json:"value"`
	Cause json.RawMessage `json:"cause"`
}

// ticket exchanges the bearer token for a short-lived WebSocket ticket.
func (c Client) ticket(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/api/auth/websocket-ticket/rpc", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", errUnauthorized
	}
	var out struct {
		Ticket string `json:"ticket"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || resp.StatusCode != http.StatusOK || out.Ticket == "" {
		return "", fmt.Errorf("t3: websocket ticket returned %d", resp.StatusCode)
	}
	return out.Ticket, nil
}

func (c Client) wsURL(ticket string) (string, error) {
	u, err := url.Parse(strings.TrimRight(c.BaseURL, "/") + "/ws")
	if err != nil {
		return "", err
	}
	u.Scheme = strings.Replace(u.Scheme, "http", "ws", 1)
	q := u.Query()
	q.Set("wsTicket", ticket)
	q.Set("orchestrationProtocol", fmt.Sprint(protocolVersion))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// call runs one RPC on its own connection. T3 writes are rare, so holding a
// socket open per instance would add state without saving anything.
func (c Client) call(ctx context.Context, method string, payload, out any) error {
	ctx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()
	ticket, err := c.ticket(ctx)
	if err != nil {
		return err
	}
	target, err := c.wsURL(ticket)
	if err != nil {
		return err
	}
	conn, resp, err := websocket.Dial(ctx, target, &websocket.DialOptions{HTTPClient: c.HTTP})
	if err != nil {
		if resp != nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
			return errUnauthorized
		}
		return fmt.Errorf("t3: websocket: %w", err)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(rpcReadLimit)
	req, err := json.Marshal(map[string]any{"_tag": "Request", "id": "1", "tag": method, "payload": payload, "headers": []any{}})
	if err != nil {
		return err
	}
	if err := conn.Write(ctx, websocket.MessageText, req); err != nil {
		return err
	}
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("t3: %s: %w", method, err)
		}
		msgs, err := decodeFrame(data)
		if err != nil {
			return err
		}
		for _, m := range msgs {
			switch m.Tag {
			case "Ping":
				if err := conn.Write(ctx, websocket.MessageText, []byte(`{"_tag":"Pong"}`)); err != nil {
					return err
				}
			case "Exit":
				return finish(method, m.Exit, out)
			case "Defect", "ClientProtocolError":
				return fmt.Errorf("t3: %s: %s", method, data)
			}
		}
	}
}

// decodeFrame accepts one message or a batch; the server may send either.
func decodeFrame(data []byte) ([]rpcMessage, error) {
	if b := bytes.TrimSpace(data); len(b) > 0 && b[0] == '[' {
		var msgs []rpcMessage
		return msgs, json.Unmarshal(b, &msgs)
	}
	var m rpcMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return []rpcMessage{m}, nil
}

func finish(method string, raw json.RawMessage, out any) error {
	var exit rpcExit
	if err := json.Unmarshal(raw, &exit); err != nil {
		return err
	}
	if exit.Tag != "Success" {
		return fmt.Errorf("t3: %s failed: %s", method, exit.Cause)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(exit.Value, out)
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// Dispatch sends an orchestration command. The command id is filled in here so
// that every caller gets T3's idempotency without generating ids.
func (c Client) Dispatch(ctx context.Context, command map[string]any) error {
	if _, ok := command["commandId"]; !ok {
		command["commandId"] = newID()
	}
	return c.call(ctx, "orchestration.dispatchCommand", command, nil)
}

// CreateThreadInput describes a new thread. An empty RuntimeMode means
// full-access, T3's default.
type CreateThreadInput struct {
	ProjectID   string
	Title       string
	InstanceID  string
	Model       string
	RuntimeMode string
	// Effort is the reasoning effort; EffortOption names the model's option
	// that holds it (reasoningEffort, effort, ...). Both empty keeps the default.
	EffortOption string
	Effort       string
}

// CreateThread creates an empty thread and returns its id.
func (c Client) CreateThread(ctx context.Context, in CreateThreadInput) (string, error) {
	id, mode := newID(), in.RuntimeMode
	if mode == "" {
		mode = "full-access"
	}
	selection := map[string]any{"instanceId": in.InstanceID, "model": in.Model}
	if in.EffortOption != "" && in.Effort != "" {
		selection["options"] = []map[string]any{{"id": in.EffortOption, "value": in.Effort}}
	}
	err := c.Dispatch(ctx, map[string]any{
		"type": "thread.create", "threadId": id, "projectId": in.ProjectID, "title": in.Title,
		"modelSelection": selection,
		"runtimeMode":    mode, "interactionMode": "default", "branch": nil, "worktreePath": nil,
		"createdBy": "user", "creationSource": "web",
	})
	return id, err
}

// SendMessage starts a turn, or queues it after the active one.
func (c Client) SendMessage(ctx context.Context, threadID, text string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("t3: empty message")
	}
	return c.Dispatch(ctx, map[string]any{
		"type": "message.dispatch", "threadId": threadID, "messageId": newID(), "text": text,
		"attachments": []any{}, "createdBy": "user", "creationSource": "web",
		"dispatchMode": map[string]any{"type": "queue_after_active"},
	})
}

// RespondApproval resolves a permission request.
func (c Client) RespondApproval(ctx context.Context, threadID, requestID, decision string) error {
	return c.Dispatch(ctx, map[string]any{"type": "runtime-request.respond", "threadId": threadID, "requestId": requestID, "decision": decision})
}

// RespondUserInput answers a structured question.
func (c Client) RespondUserInput(ctx context.Context, threadID, requestID string, answers map[string]any) error {
	return c.Dispatch(ctx, map[string]any{"type": "runtime-request.respond", "threadId": threadID, "requestId": requestID, "answers": answers})
}

// DeleteThread removes a thread.
func (c Client) DeleteThread(ctx context.Context, threadID string) error {
	return c.Dispatch(ctx, map[string]any{"type": "thread.delete", "threadId": threadID})
}

// contextMessages is how much of a thread Dory shows; T3 has the rest.
const contextMessages = 8

// Message is one recent message of a thread.
type Message struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// QuestionOption is one choice of a structured question.
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// Question is a structured question from an agent.
type Question struct {
	ID       string           `json:"id"`
	Header   string           `json:"header"`
	Question string           `json:"question"`
	Options  []QuestionOption `json:"options"`
	Multi    bool             `json:"multi_select"`
}

// Pending is the open request of a thread. Detail holds the exact command or
// action; it is empty when T3 did not expose one.
type Pending struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	Detail    string     `json:"detail"`
	Questions []Question `json:"questions"`
}

// ThreadContext is the recent state of a thread needed to answer it.
type ThreadContext struct {
	Messages []Message `json:"messages"`
	Pending  *Pending  `json:"pending"`
}

type nativeRef struct {
	NativeID string `json:"nativeId"`
}

type wireProjection struct {
	Messages []struct {
		Role string `json:"role"`
		Text string `json:"text"`
	} `json:"messages"`
	RuntimeRequests []struct {
		ID     string     `json:"id"`
		Kind   string     `json:"kind"`
		Status string     `json:"status"`
		Ref    *nativeRef `json:"nativeRequestRef"`
	} `json:"runtimeRequests"`
	TurnItems []wireItem `json:"turnItems"`
}

type wireItem struct {
	Type      string          `json:"type"`
	Ref       *nativeRef      `json:"nativeItemRef"`
	RequestID string          `json:"requestId"`
	Input     json.RawMessage `json:"input"`
	FileName  string          `json:"fileName"`
	Questions []struct {
		Question
		MultiSelect bool `json:"multiSelect"`
	} `json:"questions"`
}

func (it wireItem) detail() string {
	var text string
	switch {
	case json.Unmarshal(it.Input, &text) == nil && text != "":
		return text
	case it.FileName != "":
		return it.FileName
	default:
		return string(it.Input)
	}
}

// ThreadContext reads the last messages and the open request of a thread.
func (c Client) ThreadContext(ctx context.Context, threadID string) (ThreadContext, error) {
	var p wireProjection
	if err := c.call(ctx, "orchestration.getThreadProjection", map[string]any{"threadId": threadID}, &p); err != nil {
		return ThreadContext{}, err
	}
	return p.context(contextMessages), nil
}

// ThreadConversation reads every message and the open request of a thread.
func (c Client) ThreadConversation(ctx context.Context, threadID string) (ThreadContext, error) {
	var p wireProjection
	if err := c.call(ctx, "orchestration.getThreadProjection", map[string]any{"threadId": threadID}, &p); err != nil {
		return ThreadContext{}, err
	}
	return p.context(len(p.Messages)), nil
}

// context keeps the last limit messages.
func (p wireProjection) context(limit int) ThreadContext {
	out := ThreadContext{Messages: []Message{}}
	for _, m := range p.Messages[max(0, len(p.Messages)-limit):] {
		out.Messages = append(out.Messages, Message{Role: m.Role, Text: m.Text})
	}
	for _, r := range p.RuntimeRequests {
		if r.Status != "pending" {
			continue
		}
		pending := &Pending{ID: r.ID, Kind: r.Kind, Questions: []Question{}}
		for _, it := range p.TurnItems {
			if it.Type == "user_input_request" && it.RequestID == r.ID {
				for _, q := range it.Questions {
					q.Multi = q.MultiSelect
					pending.Questions = append(pending.Questions, q.Question)
				}
			}
			// The tool call shares its provider id with the request.
			if it.Type != "approval_request" && it.Type != "user_input_request" && it.Ref != nil && r.Ref != nil && it.Ref.NativeID == r.Ref.NativeID {
				pending.Detail = it.detail()
			}
		}
		out.Pending = pending
	}
	return out
}
