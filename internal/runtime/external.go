package runtime

import (
	"context"
	"sort"

	"github.com/migueel15/dory/internal/harness"
)

type External = harness.External

func (m *Manager) ListExternal(ctx context.Context, p Paths) []External {
	var out []External
	for _, d := range m.registry.All() {
		if d.External != nil && (m.Enabled(d.Info().ID) || d.Info().Capabilities.OfflineHistory) {
			out = append(out, d.External.List(ctx, p)...)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out
}

func (m *Manager) FindExternal(ctx context.Context, p Paths, provider, id string) (External, error) {
	if d, ok := m.registry.Get(provider); ok && d.External != nil && (m.Enabled(provider) || d.Info().Capabilities.OfflineHistory) {
		for _, e := range d.External.List(ctx, p) {
			if e.ID == id {
				return e, nil
			}
		}
	}
	return External{}, ErrNotFound.Msgf("%s session %s", provider, id)
}

func (m *Manager) ReadExternal(ctx context.Context, e External) ([]Message, error) {
	if d, ok := m.registry.Get(e.Provider); ok && d.External != nil && (m.Enabled(e.Provider) || d.Info().Capabilities.OfflineHistory) {
		return d.External.Read(ctx, e)
	}
	return nil, ErrProvider
}
