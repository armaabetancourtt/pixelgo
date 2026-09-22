package devices

import (
	"context"
	"sort"
	"sync"

	"github.com/armaabetancourtt/pixelgo/server/internal/auth"
)

type memoryRecord struct {
	device Device
	userID string
}

type MemoryRepository struct {
	mu    sync.RWMutex
	items map[string]memoryRecord
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{items: make(map[string]memoryRecord)}
}

func (r *MemoryRepository) Create(ctx context.Context, device Device) (Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[device.ID] = memoryRecord{
		device: device,
		userID: auth.UserID(ctx),
	}
	return device, nil
}

func (r *MemoryRepository) List(ctx context.Context) ([]Device, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	userID := auth.UserID(ctx)
	out := make([]Device, 0, len(r.items))
	for _, record := range r.items {
		if userID != "" && record.userID != userID {
			continue
		}
		out = append(out, record.device)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (r *MemoryRepository) Get(ctx context.Context, id string) (Device, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	record, ok := r.items[id]
	if !ok {
		return Device{}, ErrNotFound
	}
	userID := auth.UserID(ctx)
	if userID != "" && record.userID != userID {
		return Device{}, ErrNotFound
	}
	return record.device, nil
}

func (r *MemoryRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, ok := r.items[id]
	if !ok {
		return ErrNotFound
	}
	userID := auth.UserID(ctx)
	if userID != "" && record.userID != userID {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}
