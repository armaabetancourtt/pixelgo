package transfers

import (
	"context"
	"sort"
	"sync"
)

type MemoryRepository struct {
	mu    sync.RWMutex
	items map[string]Transfer
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{items: map[string]Transfer{}}
}

func (r *MemoryRepository) Create(_ context.Context, t Transfer) (Transfer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[t.ID] = t
	return t, nil
}

func (r *MemoryRepository) List(_ context.Context) ([]Transfer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Transfer, 0, len(r.items))
	for _, t := range r.items { out = append(out, t) }
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (r *MemoryRepository) Get(_ context.Context, id string) (Transfer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.items[id]
	if !ok { return Transfer{}, ErrNotFound }
	return t, nil
}

func (r *MemoryRepository) Update(_ context.Context, t Transfer) (Transfer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[t.ID]; !ok { return Transfer{}, ErrNotFound }
	r.items[t.ID] = t
	return t, nil
}
