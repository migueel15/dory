package runtime

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/migueel15/dory/internal/data"
)

// MaxActiveChildren caps the subagents of one thread working at once.
const MaxActiveChildren = 5

// resultChars caps the result a subagent hands to its parent.
const resultChars = 4000

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
	if parent.ParentID != nil {
		return data.Thread{}, fmt.Errorf("%w: a subagent cannot launch subagents", data.ErrInvalid)
	}
	if strings.TrimSpace(sub.Prompt) == "" {
		return data.Thread{}, fmt.Errorf("%w: the subagent needs a prompt", data.ErrInvalid)
	}
	working, err := m.store.Threads().List(ctx, data.Filter{Fields: map[string]any{"parent_id": parent.ID, "status": "working"}, Limit: MaxActiveChildren + 1})
	if err != nil {
		return data.Thread{}, err
	}
	if len(working) >= MaxActiveChildren {
		return data.Thread{}, fmt.Errorf("%w: %d subagents are already working; wait for one or stop it", data.ErrInvalid, MaxActiveChildren)
	}
	base := data.Thread{Provider: parent.Provider, Model: parent.Model}
	var settings ThreadSettings
	if sub.Provider != "" && !strings.EqualFold(sub.Provider, parent.Provider) {
		if sub.Model == "" {
			return data.Thread{}, fmt.Errorf("%w: choose a model for provider %s; use list_models", data.ErrInvalid, sub.Provider)
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
	if err = m.Send(ctx, child.ID, sub.Prompt); err != nil {
		return child, err
	}
	return m.store.Threads().Get(ctx, child.ID)
}

// notifyParent hands the result of a finished subagent turn to its parent,
// starting a parent turn or queueing behind the running one.
func (m *Manager) notifyParent(ctx context.Context, child data.Thread, status string, stopped bool) {
	parent, err := m.store.Threads().Get(ctx, *child.ParentID)
	if err != nil || parent.ArchivedAt != nil || parent.NodeID != m.node {
		return
	}
	state := map[string]string{"idle": "terminó", "error": "falló", "waiting": "espera una respuesta"}[status]
	if stopped {
		state = "se detuvo"
	}
	if state == "" {
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
	if r := []rune(result); len(r) > resultChars {
		result = string(r[:resultChars]) + "…"
	}
	text := fmt.Sprintf("[Subagente «%s» (%s) %s]", child.Title, child.ID, state)
	if status == "waiting" {
		text += "\nTiene una pregunta pendiente: respóndela con thread_send o déjala al usuario."
	}
	if result != "" {
		text += "\n\n" + result
	}
	if err := m.Send(ctx, parent.ID, text); err != nil {
		log.Printf("runtime: notify parent %s: %v", parent.ID, err)
	}
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
