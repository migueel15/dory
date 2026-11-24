package runtime

import (
	"context"
	"log"

	"github.com/migueel15/dory/internal/nodestate"
)

func (m *Manager) rememberCatalog(id string, st Status) []Model {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.catalogs == nil {
		var saved map[string][]Model
		if _, err := m.state.Get(context.Background(), nodestate.KeyModelCatalog, &saved); err != nil {
			log.Printf("runtime: load model catalog: %v", err)
		} else {
			m.catalogs = saved
		}
		if m.catalogs == nil {
			m.catalogs = make(map[string][]Model)
		}
	}
	if st.Catalog {
		m.catalogs[id] = st.Models
		if err := m.state.Put(context.Background(), nodestate.KeyModelCatalog, m.catalogs); err != nil {
			log.Printf("runtime: save model catalog: %v", err)
		}
		return st.Models
	}
	if saved := m.catalogs[id]; len(saved) > 0 {
		return saved
	}
	return st.Models
}
