package presence

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type Store interface {
	Touch(ctx context.Context, deviceID, leaseID string, ttl time.Duration) error
	Remove(ctx context.Context, deviceID, leaseID string) error
	Online(ctx context.Context, deviceIDs []string) (map[string]bool, error)
}

type MemoryStore struct {
	mu     sync.Mutex
	leases map[string]map[string]time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{leases: make(map[string]map[string]time.Time)}
}

func (s *MemoryStore) Touch(_ context.Context, deviceID, leaseID string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneLocked(time.Now())
	if s.leases[deviceID] == nil {
		s.leases[deviceID] = make(map[string]time.Time)
	}
	s.leases[deviceID][leaseID] = time.Now().Add(ttl)
	return nil
}

func (s *MemoryStore) Remove(_ context.Context, deviceID, leaseID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if leases := s.leases[deviceID]; leases != nil {
		delete(leases, leaseID)
		if len(leases) == 0 {
			delete(s.leases, deviceID)
		}
	}
	return nil
}

func (s *MemoryStore) Online(_ context.Context, deviceIDs []string) (map[string]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneLocked(time.Now())
	out := make(map[string]bool, len(deviceIDs))
	for _, deviceID := range deviceIDs {
		out[deviceID] = len(s.leases[deviceID]) > 0
	}
	return out, nil
}

func (s *MemoryStore) pruneLocked(now time.Time) {
	for deviceID, leases := range s.leases {
		for leaseID, expiry := range leases {
			if !expiry.After(now) {
				delete(leases, leaseID)
			}
		}
		if len(leases) == 0 {
			delete(s.leases, deviceID)
		}
	}
}

type RedisStore struct {
	client *redis.Client
	prefix string
}

func NewRedisStore(redisURL string) (*RedisStore, error) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(options)
	return &RedisStore{client: client, prefix: "pixelgo:presence:"}, nil
}

func (s *RedisStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

func (s *RedisStore) Close() error {
	return s.client.Close()
}

func (s *RedisStore) Touch(ctx context.Context, deviceID, leaseID string, ttl time.Duration) error {
	now := time.Now()
	expiry := now.Add(ttl)
	key := s.key(deviceID)

	pipe := s.client.TxPipeline()
	pipe.ZRemRangeByScore(ctx, key, "-inf", formatScore(now))
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(expiry.UnixMilli()), Member: leaseID})
	pipe.Expire(ctx, key, ttl*2)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *RedisStore) Remove(ctx context.Context, deviceID, leaseID string) error {
	return s.client.ZRem(ctx, s.key(deviceID), leaseID).Err()
}

func (s *RedisStore) Online(ctx context.Context, deviceIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(deviceIDs))
	if len(deviceIDs) == 0 {
		return out, nil
	}

	now := time.Now()
	pipe := s.client.Pipeline()
	counts := make(map[string]*redis.IntCmd, len(deviceIDs))

	for _, deviceID := range deviceIDs {
		key := s.key(deviceID)
		pipe.ZRemRangeByScore(ctx, key, "-inf", formatScore(now))
		counts[deviceID] = pipe.ZCard(ctx, key)
	}

	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	for deviceID, command := range counts {
		count, err := command.Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return nil, err
		}
		out[deviceID] = count > 0
	}
	return out, nil
}

func (s *RedisStore) key(deviceID string) string {
	return s.prefix + deviceID
}

func formatScore(t time.Time) string {
	return time.UnixMilli(t.UnixMilli()).Format("20060102150405.000")
}
