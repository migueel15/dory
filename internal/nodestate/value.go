package nodestate

import (
	"context"
	"encoding/json"
	"sync"
)

// Value caches a typed key. Get returns a read-only snapshot; use Set or
// Update to change it. Snapshots containing references must be treated as immutable.
type Value[T any] struct {
	store    *Store
	key      string
	defaults T
	mu       sync.RWMutex
	current  T
}

func NewValue[T any](store *Store, key string, defaults T) *Value[T] {
	return &Value[T]{store: store, key: key, defaults: defaults, current: defaults}
}

func (v *Value[T]) Load(ctx context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	next, err := v.fresh()
	if err != nil {
		return err
	}
	if _, err := v.store.Get(ctx, v.key, &next); err != nil {
		return err
	}
	v.current = next
	return nil
}

func (v *Value[T]) Get() T {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.current
}

func (v *Value[T]) Set(ctx context.Context, next T) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.store.Put(ctx, v.key, next); err != nil {
		return err
	}
	v.current = next
	return nil
}

func (v *Value[T]) Update(ctx context.Context, fn func(*T)) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	next, err := v.fresh()
	if err != nil {
		return err
	}
	if err := v.store.Update(ctx, v.key, &next, func() error {
		fn(&next)
		return nil
	}); err != nil {
		return err
	}
	v.current = next
	return nil
}

// fresh keeps decoding and callbacks from mutating defaults that contain maps,
// slices, or pointers shared with an earlier snapshot.
func (v *Value[T]) fresh() (T, error) {
	var next T
	raw, err := json.Marshal(v.defaults)
	if err != nil {
		return next, err
	}
	err = json.Unmarshal(raw, &next)
	return next, err
}
