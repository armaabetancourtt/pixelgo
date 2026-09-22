package presence

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type Store interface {
	Online(ctx context.Context, deviceID string, ttl time.Duration) error
	Offline(ctx context.Context, deviceID string) error
	IsOnline(ctx context.Context, deviceID string) (bool, error)
}

type MemoryStore struct {
	mu      sync.RWMutex
	expires map[string]time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{expires: make(map[string]time.Time)}
}

func (s *MemoryStore) Online(_ context.Context, deviceID string, ttl time.Duration) error {
	s.mu.Lock()
	s.expires[deviceID] = time.Now().Add(ttl)
	s.mu.Unlock()
	return nil
}

func (s *MemoryStore) Offline(_ context.Context, deviceID string) error {
	s.mu.Lock()
	delete(s.expires, deviceID)
	s.mu.Unlock()
	return nil
}

func (s *MemoryStore) IsOnline(_ context.Context, deviceID string) (bool, error) {
	s.mu.RLock()
	expiresAt, ok := s.expires[deviceID]
	s.mu.RUnlock()
	return ok && time.Now().Before(expiresAt), nil
}

type RedisStore struct {
	client *redis.Client
	prefix string
}

func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{
		client: client,
		prefix: "pixelgo:presence:",
	}
}

func (s *RedisStore) Online(ctx context.Context, deviceID string, ttl time.Duration) error {
	return s.client.Set(ctx, s.key(deviceID), time.Now().UTC().Format(time.RFC3339Nano), ttl).Err()
}

func (s *RedisStore) Offline(ctx context.Context, deviceID string) error {
	return s.client.Del(ctx, s.key(deviceID)).Err()
}

func (s *RedisStore) IsOnline(ctx context.Context, deviceID string) (bool, error) {
	count, err := s.client.Exists(ctx, s.key(deviceID)).Result()
	return count > 0, err
}

func (s *RedisStore) key(deviceID string) string {
	return s.prefix + deviceID
}
