package devices

import (
	"context"
	"sort"
	"sync"
)

type MemoryRepository struct {
	mu    sync.RWMutex
	items map[string]Device
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{items: make(map[string]Device)}
}

func (r *MemoryRepository) Create(_ context.Context, device Device) (Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[device.ID] = device
	return device, nil
}

func (r *MemoryRepository) List(_ context.Context) ([]Device, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Device, 0, len(r.items))
	for _, device := range r.items {
		out = append(out, device)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (r *MemoryRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}
