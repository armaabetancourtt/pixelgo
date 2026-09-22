package auth

import (
	"context"
	"sync"
	"time"
)

type memoryRefresh struct {
	userID    string
	familyID  string
	tokenHash string
	expiresAt time.Time
	usedAt    *time.Time
	revokedAt *time.Time
}

type MemoryRepository struct {
	mu       sync.Mutex
	users    map[string]User
	refresh  map[string]*memoryRefresh
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		users:   make(map[string]User),
		refresh: make(map[string]*memoryRefresh),
	}
}

func (r *MemoryRepository) CreateUser(_ context.Context, email, passwordHash string) (User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.users[email]; exists {
		return User{}, ErrEmailTaken
	}
	id, err := randomHex(16)
	if err != nil {
		return User{}, err
	}
	user := User{
		ID:           "usr_memory_" + id,
		Email:        email,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now().UTC(),
	}
	r.users[email] = user
	return user, nil
}

func (r *MemoryRepository) GetUserByEmail(_ context.Context, email string) (User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	user, ok := r.users[email]
	if !ok {
		return User{}, ErrInvalidCredentials
	}
	return user, nil
}

func (r *MemoryRepository) CreateRefresh(_ context.Context, userID, familyID, tokenHash string, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refresh[tokenHash] = &memoryRefresh{
		userID: userID, familyID: familyID, tokenHash: tokenHash, expiresAt: expiresAt,
	}
	return nil
}

func (r *MemoryRepository) RotateRefresh(_ context.Context, tokenHash, replacementHash string, replacementExpiresAt, now time.Time) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, ok := r.refresh[tokenHash]
	if !ok || record.revokedAt != nil || !record.expiresAt.After(now) {
		return "", ErrInvalidRefresh
	}
	if record.usedAt != nil {
		for _, item := range r.refresh {
			if item.familyID == record.familyID && item.revokedAt == nil {
				revoked := now
				item.revokedAt = &revoked
			}
		}
		return "", ErrRefreshReuse
	}

	used := now
	record.usedAt = &used
	r.refresh[replacementHash] = &memoryRefresh{
		userID: record.userID, familyID: record.familyID, tokenHash: replacementHash, expiresAt: replacementExpiresAt,
	}
	return record.userID, nil
}
