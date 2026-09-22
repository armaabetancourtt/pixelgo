package transfers

import (
	"context"
	"sort"
	"sync"

	"github.com/armaabetancourtt/pixelgo/server/internal/auth"
)

type memoryRecord struct {
	transfer Transfer
	userID   string
}

type MemoryRepository struct {
	mu    sync.RWMutex
	items map[string]memoryRecord
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{items: map[string]memoryRecord{}}
}

func (r *MemoryRepository) Create(ctx context.Context, t Transfer) (Transfer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[t.ID] = memoryRecord{
		transfer: t,
		userID:   auth.UserID(ctx),
	}
	return t, nil
}

func (r *MemoryRepository) List(ctx context.Context) ([]Transfer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	userID := auth.UserID(ctx)
	out := make([]Transfer, 0, len(r.items))
	for _, record := range r.items {
		if userID != "" && record.userID != userID {
			continue
		}
		out = append(out, record.transfer)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (r *MemoryRepository) Get(ctx context.Context, id string) (Transfer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	record, ok := r.items[id]
	if !ok {
		return Transfer{}, ErrNotFound
	}
	userID := auth.UserID(ctx)
	if userID != "" && record.userID != userID {
		return Transfer{}, ErrNotFound
	}
	return record.transfer, nil
}

func (r *MemoryRepository) Update(ctx context.Context, t Transfer) (Transfer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, ok := r.items[t.ID]
	if !ok {
		return Transfer{}, ErrNotFound
	}
	userID := auth.UserID(ctx)
	if userID != "" && record.userID != userID {
		return Transfer{}, ErrNotFound
	}
	record.transfer = t
	r.items[t.ID] = record
	return t, nil
}
