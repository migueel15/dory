package harness

import (
	"cmp"
	"context"
	"log"
	"slices"
	"sync"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/harness/procpool"
	"github.com/migueel15/dory/internal/nodestate"
)

var ErrProvider = apperr.New(apperr.Invalid, "provider not available")

type Registry struct {
	mu        sync.RWMutex
	providers map[string]EntryPoint
	dataDir   string
	pool      *procpool.Pool
	accounts  *nodestate.Value[[]Account]
	disabled  *nodestate.Value[disabledProviders]
}

func NewRegistry(state *nodestate.Store, dataDir string, pool *procpool.Pool, drivers ...Driver) *Registry {
	r := &Registry{
		providers: map[string]EntryPoint{}, dataDir: dataDir, pool: pool,
		accounts: nodestate.NewValue[[]Account](state, nodestate.KeyAccounts, nil),
		disabled: nodestate.NewValue(state, nodestate.KeyDisabledProviders, disabledProviders{}),
	}
	for _, d := range drivers {
		r.Register(d)
	}
	if err := r.disabled.Load(context.Background()); err != nil {
		log.Printf("harness: load disabled providers: %v", err)
	}
	if err := r.accounts.Load(context.Background()); err != nil {
		log.Printf("harness: load accounts: %v", err)
	}
	for _, a := range r.accounts.Get() {
		if base := r.providers[a.Driver]; base.Accounts != nil {
			r.providers[a.ID()] = entry(base.Accounts.WithAccount(a))
		}
	}
	return r
}

func (r *Registry) Register(d Driver) {
	if p, ok := d.(pooled); ok {
		d = p.WithPool(r.pool)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[d.Info().ID] = entry(d)
}

func (r *Registry) Get(id string) (EntryPoint, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.providers[id]
	return d, ok
}

// All returns a snapshot including disabled accounts, ordered by driver and id.
func (r *Registry) All() []EntryPoint {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]EntryPoint, 0, len(r.providers))
	for _, d := range r.providers {
		out = append(out, d)
	}
	slices.SortFunc(out, func(a, b EntryPoint) int {
		x, y := a.Info(), b.Info()
		if order := cmp.Compare(x.Order, y.Order); order != 0 {
			return order
		}
		return cmp.Compare(x.ID, y.ID)
	})
	return out
}

func (r *Registry) Capabilities(id string) Capabilities {
	if d, ok := r.Get(id); ok {
		return d.Info().Capabilities
	}
	return Capabilities{}
}

type disabledProviders struct {
	Disabled []string `json:"disabled"`
}

// Enabled says whether a provider is turned on on this node.
func (r *Registry) Enabled(id string) bool {
	return !slices.Contains(r.disabled.Get().Disabled, id)
}

// SetEnabled turns a provider on or off on this node and saves the choice.
func (r *Registry) SetEnabled(ctx context.Context, id string, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.providers[id].Driver == nil {
		return ErrProvider.Msgf("%s", id)
	}
	if err := r.disabled.Update(ctx, func(saved *disabledProviders) {
		if enabled {
			saved.Disabled = slices.DeleteFunc(saved.Disabled, func(d string) bool { return d == id })
		} else if !slices.Contains(saved.Disabled, id) {
			saved.Disabled = append(saved.Disabled, id)
			slices.Sort(saved.Disabled)
		}
	}); err != nil {
		return err
	}
	return nil
}
