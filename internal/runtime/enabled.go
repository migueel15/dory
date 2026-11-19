package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
)

// A provider can be turned off on a node: it then disappears there. It is not
// listed for new threads, its threads cannot run turns and its usage is not
// asked for. The setting belongs to the node and is not replicated.

func (m *Manager) disabledFile() string { return filepath.Join(m.dataDir, "providers.json") }

func (m *Manager) loadDisabled() {
	m.disabled = map[string]bool{}
	if m.dataDir == "" {
		return
	}
	raw, err := os.ReadFile(m.disabledFile())
	if err != nil {
		return
	}
	var saved struct {
		Disabled []string `json:"disabled"`
	}
	if json.Unmarshal(raw, &saved) == nil {
		for _, id := range saved.Disabled {
			m.disabled[id] = true
		}
	}
}

// Enabled says whether a provider is turned on on this node.
func (m *Manager) Enabled(id string) bool {
	m.disabledMu.Lock()
	defer m.disabledMu.Unlock()
	return !m.disabled[id]
}

// SetEnabled turns a provider on or off on this node and saves the choice.
func (m *Manager) SetEnabled(ctx context.Context, id string, enabled bool) error {
	if _, ok := m.providers[id]; !ok {
		return fmt.Errorf("%w: %s", ErrProvider, id)
	}
	m.disabledMu.Lock()
	if enabled {
		delete(m.disabled, id)
	} else {
		m.disabled[id] = true
	}
	ids := make([]string, 0, len(m.disabled))
	for d := range m.disabled {
		ids = append(ids, d)
	}
	m.disabledMu.Unlock()
	sort.Strings(ids)
	if m.dataDir != "" {
		raw, _ := json.Marshal(map[string][]string{"disabled": ids})
		tmp := m.disabledFile() + ".tmp"
		if err := os.WriteFile(tmp, raw, 0o600); err != nil {
			return err
		}
		if err := os.Rename(tmp, m.disabledFile()); err != nil {
			return err
		}
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
	p, ok := m.providers[id]
	if !ok || !m.Enabled(id) {
		return nil, false
	}
	return p, true
}
