package runtime

import (
	"context"
	"log"
)

func (m *Manager) Enabled(id string) bool { return m.registry.Enabled(id) }
func (m *Manager) SetEnabled(ctx context.Context, id string, enabled bool) error {
	if err := m.registry.SetEnabled(ctx, id, enabled); err != nil {
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

func (m *Manager) provider(id string) (Provider, bool) {
	p, ok := m.registry.Get(id)
	return p.Driver, ok && m.Enabled(id)
}
