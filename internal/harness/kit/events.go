package kit

import (
	"strings"

	"github.com/migueel15/dory/internal/harness"
)

// Text collects fragments for the final event while publishing each fragment once.
type Text struct {
	ID      string
	Kind    harness.TextKind
	Meta    harness.ItemMeta
	content strings.Builder
}

func NewText(id, run string, kind harness.TextKind) *Text {
	return &Text{ID: id, Kind: kind, Meta: harness.ItemMeta{RunID: run, CreatedAt: Now()}}
}

func (t *Text) Append(fragment string, emit func(harness.Event)) {
	if fragment == "" {
		return
	}
	t.content.WriteString(fragment)
	emit(harness.TextDelta{ItemID: t.ID, Kind: t.Kind, Delta: fragment, Meta: t.Meta})
}

func (t *Text) String() string { return t.content.String() }

func (t *Text) Done(text string) harness.TextDone {
	return harness.TextDone{ItemID: t.ID, Kind: t.Kind, Text: text, Meta: t.Meta}
}

func Plan(m *harness.Message) harness.PlanUpdated {
	snapshot := CopyOf(m)
	return harness.PlanUpdated{ItemID: m.ID, Todos: snapshot.Tool.Todos, Text: m.Text,
		Title: m.Title, Detail: m.Detail, Call: snapshot.Tool, Meta: itemMeta(m)}
}

func StartTool(m *harness.Message) harness.Event {
	if m.Role == "plan" {
		return Plan(m)
	}
	if m.Tool == nil || m.Text != "" {
		return UpdateTool(m)
	}
	snapshot := CopyOf(m)
	return harness.ToolStarted{ItemID: m.ID, Call: *snapshot.Tool, Title: m.Title, Detail: m.Detail,
		InputChars: m.InputChars, Meta: itemMeta(m)}
}

func UpdateTool(m *harness.Message) harness.ToolUpdated {
	snapshot := CopyOf(m)
	return harness.ToolUpdated{ItemID: m.ID, Call: snapshot.Tool, Title: m.Title, Detail: m.Detail,
		Status: m.Status, Result: m.Text, InputChars: m.InputChars, ResultChars: m.ResultChars,
		Streaming: m.Streaming, Meta: itemMeta(m)}
}

func FinishTool(m *harness.Message) harness.Event {
	if m.Role == "plan" {
		return Plan(m)
	}
	update := UpdateTool(m)
	return harness.ToolDone{ItemID: m.ID, Status: m.Status, Result: m.Text,
		ResultChars: m.ResultChars, Update: &update}
}

func itemMeta(m *harness.Message) harness.ItemMeta {
	return harness.ItemMeta{RunID: m.RunID, CreatedAt: m.CreatedAt}
}
