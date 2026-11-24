package runtime

import (
	"context"
	"log"
	"slices"
)

// A provider can be turned off on a node: it then disappears there. It is not
// listed for new threads, its threads cannot run turns and its usage is not
// asked for. The setting belongs to the node and is not replicated.

type disabledProviders struct {
	Disabled []string `json:"disabled"`
}

// Enabled says whether a provider is turned on on this node.
func (m *Manager) Enabled(id string) bool {
	return !slices.Contains(m.disabled.Get().Disabled, id)
}

// SetEnabled turns a provider on or off on this node and saves the choice.
func (m *Manager) SetEnabled(ctx context.Context, id string, enabled bool) error {
	if m.provider0(id) == nil {
		return ErrProvider.Msgf("%s", id)
	}
	if err := m.disabled.Update(ctx, func(saved *disabledProviders) {
		if enabled {
			saved.Disabled = slices.DeleteFunc(saved.Disabled, func(d string) bool { return d == id })
		} else if !slices.Contains(saved.Disabled, id) {
			saved.Disabled = append(saved.Disabled, id)
			slices.Sort(saved.Disabled)
		}
	}); err != nil {
		return err
	}
	m.invalidate()
	if enabled {
		go func() {
			m.RefreshUsage(context.WithoutCancel(ctx))
			log.Printf("runtime: %s enabled", id)
		}()
	}
	return nil
}

// provider returns a registered provider that is turned on.
func (m *Manager) provider(id string) (Provider, bool) {
	p := m.provider0(id)
	if p == nil || !m.Enabled(id) {
		return nil, false
	}
	return p, true
}
