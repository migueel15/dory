package acp

import "github.com/migueel15/dory/internal/harness"

// transcript folds typed events back into the messages they describe, in the
// order they first appeared. It serves replays of saved sessions.
type transcript struct {
	index    map[string]int
	messages []harness.Message
}

func (t *transcript) put(msg harness.Message) *harness.Message {
	if t.index == nil {
		t.index = map[string]int{}
	}
	i, ok := t.index[msg.ID]
	if !ok {
		i = len(t.messages)
		t.index[msg.ID] = i
		t.messages = append(t.messages, msg)
	}
	return &t.messages[i]
}

func (t *transcript) item(id, role string, meta harness.ItemMeta) *harness.Message {
	m := t.put(harness.Message{ID: id, Role: role})
	if meta.RunID != "" {
		m.RunID = meta.RunID
	}
	if meta.CreatedAt != "" {
		m.CreatedAt = meta.CreatedAt
	}
	return m
}

func (t *transcript) tool(e harness.ToolUpdated) *harness.Message {
	m := t.item(e.ItemID, "tool", e.Meta)
	m.Tool, m.Title, m.Detail, m.Status, m.Text = e.Call, e.Title, e.Detail, e.Status, e.Result
	m.InputChars, m.ResultChars, m.Streaming = e.InputChars, e.ResultChars, e.Streaming
	return m
}

func (t *transcript) apply(event harness.Event) {
	switch e := event.(type) {
	case harness.TextDelta:
		m := t.item(e.ItemID, string(e.Kind), e.Meta)
		if e.Replace {
			m.Text = ""
		}
		m.Text += e.Delta
		m.Streaming = true
	case harness.TextDone:
		m := t.item(e.ItemID, string(e.Kind), e.Meta)
		m.Text, m.Streaming = e.Text, false
	case harness.ToolStarted:
		m := t.item(e.ItemID, "tool", e.Meta)
		m.Tool, m.Title, m.Detail, m.InputChars, m.Status = &e.Call, e.Title, e.Detail, e.InputChars, "running"
	case harness.ToolUpdated:
		t.tool(e)
	case harness.ToolDone:
		var m *harness.Message
		if e.Update != nil {
			m = t.tool(*e.Update)
		} else {
			m = t.item(e.ItemID, "tool", harness.ItemMeta{})
		}
		if e.Call != nil {
			m.Tool = e.Call
		}
		m.Text, m.Status, m.ResultChars, m.Streaming = e.Result, e.Status, e.ResultChars, false
	case harness.PlanUpdated:
		m := t.item(e.ItemID, "plan", e.Meta)
		m.Text, m.Title, m.Detail, m.Tool = e.Text, e.Title, e.Detail, e.Call
	case harness.MessageReported:
		*t.put(e.Message) = e.Message
	}
}
