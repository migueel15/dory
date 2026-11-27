package runtime

import (
	"context"
	"log"

	"github.com/migueel15/dory/internal/nodestate"
)

func (m *Manager) rememberCatalog(id string, st Status) ([]Model, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.catalogs == nil {
		var saved map[string][]Model
		if m.state != nil {
			if _, err := m.state.Get(context.Background(), nodestate.KeyModelCatalog, &saved); err != nil {
				log.Printf("runtime: load model catalog: %v", err)
			} else {
				m.catalogs = saved
			}
		}
		if m.catalogs == nil {
			m.catalogs = make(map[string][]Model)
		}
	}
	if st.Catalog {
		m.catalogs[id] = st.Models
		if st.Driver != "" {
			m.catalogs["driver:"+st.Driver] = st.Models
		}
		if m.state != nil {
			if err := m.state.Put(context.Background(), nodestate.KeyModelCatalog, m.catalogs); err != nil {
				log.Printf("runtime: save model catalog: %v", err)
			}
		}
		return st.Models, true
	}
	if saved := m.catalogs[id]; len(saved) > 0 {
		return saved, true
	}
	if st.Driver != "" {
		if saved := m.catalogs[st.Driver]; len(saved) > 0 {
			return saved, true
		}
		if saved := m.catalogs["driver:"+st.Driver]; len(saved) > 0 {
			return saved, true
		}
	}
	return st.Models, false
}
