package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/migueel15/dory/internal/data"
)

// Applied says when a configuration takes effect.
const (
	AppliedNow       = "now"
	AppliedAfterTurn = "after_turn"
)

// ConfigureThread changes a thread for an agent. A running thread, usually the
// agent's own, keeps its turn: the change waits for the turn to end and then
// applies before the next queued message starts.
func (m *Manager) ConfigureThread(ctx context.Context, id string, settings ThreadSettings) (data.Thread, string, error) {
	t, err := m.local(ctx, id)
	if err != nil {
		return t, "", err
	}
	if err = m.ResolveSettings(ctx, t, &settings); err != nil {
		return t, "", err
	}
	m.mu.Lock()
	if _, busy := m.running[id]; busy {
		m.pending[id] = mergeSettings(m.pending[id], settings)
		m.mu.Unlock()
		m.publish(Update{ThreadID: id, Reload: true})
		return t, AppliedAfterTurn, nil
	}
	t, err = m.configureLocked(ctx, id, settings, true)
	m.mu.Unlock()
	return t, AppliedNow, err
}

// Pending returns the settings waiting for the running turn of a thread to end.
func (m *Manager) Pending(id string) *ThreadSettings {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.pending[id]; ok {
		return &s
	}
	return nil
}

// ResolveSettings checks a selection against the harnesses of this node and
// turns a model or effort name into the slug the harness expects.
func (m *Manager) ResolveSettings(ctx context.Context, t data.Thread, s *ThreadSettings) error {
	if s.RuntimeMode != nil {
		return fmt.Errorf("%w: agents cannot change the runtime mode", data.ErrInvalid)
	}
	provider := t.Provider
	if s.Provider != nil {
		provider = strings.ToLower(strings.TrimSpace(*s.Provider))
		s.Provider = &provider
	}
	var status *Status
	for _, st := range m.Providers(ctx) {
		if st.ID == provider {
			status = &st
			break
		}
	}
	if status == nil || !status.Installed {
		return fmt.Errorf("%w: %s; use list_models", ErrProvider, provider)
	}
	if !status.Enabled {
		return fmt.Errorf("%w: %s is turned off on this node", data.ErrInvalid, provider)
	}
	model := t.Model
	if s.Model != nil {
		found, ok := findModel(status.Models, *s.Model)
		if !ok {
			return fmt.Errorf("%w: model %q is not offered by %s; valid: %s", data.ErrInvalid, *s.Model, provider, modelSlugs(status.Models))
		}
		model = found.Slug
		s.Model = &model
	}
	if s.Effort != nil && *s.Effort != "" {
		if found, ok := findModel(status.Models, model); ok && len(found.Efforts) > 0 {
			effort := strings.ToLower(strings.TrimSpace(*s.Effort))
			valid := false
			for _, e := range found.Efforts {
				valid = valid || e == effort
			}
			if !valid {
				return fmt.Errorf("%w: effort %q is not valid for %s; valid: %s", data.ErrInvalid, *s.Effort, model, strings.Join(found.Efforts, ", "))
			}
			s.Effort = &effort
		}
	}
	return nil
}

// findModel matches a slug or a display name, ignoring case. Harnesses
// without a catalog accept any model.
func findModel(models []Model, name string) (Model, bool) {
	name = strings.TrimSpace(name)
	if len(models) == 0 {
		return Model{Slug: name}, name != ""
	}
	for _, md := range models {
		if strings.EqualFold(md.Slug, name) || strings.EqualFold(md.Name, name) {
			return md, true
		}
	}
	return Model{}, false
}

func modelSlugs(models []Model) string {
	out := make([]string, len(models))
	for i, md := range models {
		out[i] = md.Slug
	}
	return strings.Join(out, ", ")
}

func mergeSettings(old, next ThreadSettings) ThreadSettings {
	if next.Provider != nil {
		old.Provider = next.Provider
		// A model chosen for the previous provider does not carry over.
		old.Model, old.Effort = nil, nil
	}
	if next.Model != nil {
		old.Model = next.Model
	}
	if next.Effort != nil {
		old.Effort = next.Effort
	}
	return old
}
