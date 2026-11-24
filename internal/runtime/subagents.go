package runtime

import (
	"context"
	"regexp"
	"strings"

	"github.com/migueel15/dory/internal/data"
)

// MaxActiveChildren caps the subagents of one thread working at once.
const MaxActiveChildren = 5

// resultChars caps the result a subagent hands to its parent.
const resultChars = 4000

// harnessMarkers are inline markers a harness leaves in its answers, such as
// codex web citations between U+E200 and U+E201; they are not text.
var harnessMarkers = regexp.MustCompile("\uE200[^\uE201]*\uE201")

// Subagent describes a child thread to launch.
type Subagent struct {
	Prompt, Title           string
	Provider, Model, Effort string
}

// Spawn creates a child of parentID in the same directory, task and
// permission mode, and starts it with the prompt. The child does not see the
// parent's history. A child cannot launch children of its own.
func (m *Manager) Spawn(ctx context.Context, parentID string, sub Subagent) (data.Thread, error) {
	parent, err := m.local(ctx, parentID)
	if err != nil {
		return parent, err
	}
	if strings.TrimSpace(sub.Prompt) == "" {
		return data.Thread{}, data.ErrInvalid.Msgf("the subagent needs a prompt")
	}
	if err = m.CanSpawn(ctx, parent); err != nil {
		return data.Thread{}, err
	}
	base := data.Thread{Provider: parent.Provider, Model: parent.Model}
	var settings ThreadSettings
	if sub.Provider != "" && !strings.EqualFold(sub.Provider, parent.Provider) {
		if sub.Model == "" {
			return data.Thread{}, data.ErrInvalid.Msgf("choose a model for provider %s; use list_models", sub.Provider)
		}
		settings.Provider = &sub.Provider
	}
	if sub.Model != "" {
		settings.Model = &sub.Model
	}
	if sub.Effort != "" {
		settings.Effort = &sub.Effort
	}
	if err = m.ResolveSettings(ctx, base, &settings); err != nil {
		return data.Thread{}, err
	}
	n := NewThread{ProjectID: parent.ProjectID, TaskID: parent.TaskID, ParentID: &parent.ID, Provider: parent.Provider,
		Model: parent.Model, RuntimeMode: parent.RuntimeMode, Title: sub.Title, Branch: parent.Branch}
	if settings.Provider != nil {
		n.Provider = *settings.Provider
	}
	if settings.Model != nil {
		n.Model = *settings.Model
	}
	if settings.Effort != nil {
		n.Effort = *settings.Effort
	} else if n.Provider == parent.Provider && n.Model == parent.Model {
		n.Effort = parent.Effort
	}
	if n.Title == "" {
		n.Title = firstLine(sub.Prompt, 60)
	}
	if n.ExistingDir, err = m.Dir(ctx, parent.ID); err != nil {
		return data.Thread{}, err
	}
	child, err := m.Create(ctx, n)
	if err != nil {
		return child, err
	}
	if _, err = m.RecordDelegation(ctx, parent.ID, child.ID, m.node, child.Title); err != nil {
		return child, err
	}
	if err = m.Send(ctx, child.ID, sub.Prompt); err != nil {
		return child, err
	}
	return m.store.Threads().Get(ctx, child.ID)
}

// CanSpawn says whether parent may launch one more subagent: a subagent
// cannot launch its own, and at most MaxActiveChildren work at once.
func (m *Manager) CanSpawn(ctx context.Context, parent data.Thread) error {
	if parent.ParentID != nil {
		return data.ErrInvalid.Msgf("a subagent cannot launch subagents")
	}
	working, err := m.store.Threads().List(ctx, data.Filter{Fields: map[string]any{"parent_id": parent.ID, "status": "working"}, Limit: MaxActiveChildren + 1})
	if err != nil {
		return err
	}
	if len(working) >= MaxActiveChildren {
		return data.ErrInvalid.Msgf("%d subagents are already working; wait for one or stop it", MaxActiveChildren)
	}
	return nil
}

// notifyParent hands the outcome of a finished subagent turn to the node of
// its parent, which records it and wakes the parent.
func (m *Manager) notifyParent(ctx context.Context, child data.Thread, status string, stopped bool) {
	parent, err := m.store.Threads().Get(ctx, *child.ParentID)
	if err != nil || parent.ArchivedAt != nil {
		return
	}
	outcome := map[string]string{"idle": "completed", "error": "failed", "waiting": "waiting"}[status]
	if stopped {
		outcome = "interrupted"
	}
	if outcome == "" {
		return
	}
	var result string
	if msgs, err := m.Messages(ctx, child.ID); err == nil {
		for _, msg := range msgs {
			if msg.Role == "assistant" {
				result = msg.Text
			}
		}
	}
	result = strings.TrimSpace(harnessMarkers.ReplaceAllString(result, ""))
	if r := []rune(result); len(r) > resultChars {
		result = string(r[:resultChars]) + "…"
	}
	if fresh, err := m.store.Threads().Get(ctx, child.ID); err == nil {
		child = fresh
	}
	m.deliver(ctx, parent.NodeID, Outcome{ParentThreadID: parent.ID, ChildThreadID: child.ID, ChildNodeID: m.node, Title: child.Title, Status: outcome, Result: result})
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
