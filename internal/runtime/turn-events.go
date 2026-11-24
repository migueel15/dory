package runtime

import (
	"time"

	"github.com/migueel15/dory/internal/harness"
)

type turnMessages struct {
	run   string
	items map[string]Message
}

func (t *turnMessages) item(id, role string, meta harness.ItemMeta) Message {
	msg, ok := t.items[id]
	if !ok {
		msg = Message{ID: id, RunID: t.run, Role: role, CreatedAt: now()}
	}
	if meta.RunID != "" {
		msg.RunID = meta.RunID
	}
	if meta.CreatedAt != "" {
		msg.CreatedAt = meta.CreatedAt
	}
	return msg
}

func (t *turnMessages) toolUpdate(event harness.ToolUpdated) Message {
	msg := t.item(event.ItemID, "tool", event.Meta)
	msg.Tool, msg.Title, msg.Detail, msg.Status, msg.Text = event.Call, event.Title, event.Detail, event.Status, event.Result
	msg.InputChars, msg.ResultChars, msg.Streaming = event.InputChars, event.ResultChars, event.Streaming
	return msg
}

func (t *turnMessages) apply(event harness.Event) (Message, bool) {
	var msg Message
	switch e := event.(type) {
	case harness.TextDelta:
		msg = t.item(e.ItemID, string(e.Kind), e.Meta)
		if e.Replace {
			msg.Text = ""
		}
		msg.Text += e.Delta
		msg.Streaming = true
	case harness.TextDone:
		msg = t.item(e.ItemID, string(e.Kind), e.Meta)
		msg.Text, msg.Streaming = e.Text, false
	case harness.ToolStarted:
		msg = t.item(e.ItemID, "tool", e.Meta)
		msg.Tool, msg.Title, msg.Detail, msg.InputChars = &e.Call, e.Title, e.Detail, e.InputChars
		msg.Status = "running"
	case harness.ToolUpdated:
		msg = t.toolUpdate(e)
	case harness.ToolDone:
		msg = t.item(e.ItemID, "tool", harness.ItemMeta{})
		if e.Update != nil {
			msg = t.toolUpdate(*e.Update)
		}
		if e.Call != nil {
			msg.Tool = e.Call
		}
		msg.Text, msg.Status, msg.ResultChars, msg.Streaming = e.Result, e.Status, e.ResultChars, false
	case harness.PlanUpdated:
		msg = t.item(e.ItemID, "plan", e.Meta)
		msg.Text, msg.Title, msg.Detail, msg.Tool = e.Text, e.Title, e.Detail, e.Call
		if msg.Tool == nil {
			msg.Tool = &ToolCall{Kind: "plan", Name: "plan", Todos: e.Todos}
		}
	case harness.MessageReported:
		msg = e.Message
	default:
		return Message{}, false
	}
	t.items[msg.ID] = msg
	return msg, true
}

func (m *Manager) CloseLive(thread string) { m.pool.CloseThread(thread) }

// Checkpoint sizes double so the sum of full snapshots stays linear. The
// interval also limits writes when fragments arrive in a short burst.
type textCheckpoint struct {
	at   time.Time
	size int
}

func (c textCheckpoint) due(at time.Time, size int) bool {
	return at.Sub(c.at) >= 500*time.Millisecond && size-c.size >= c.size
}
