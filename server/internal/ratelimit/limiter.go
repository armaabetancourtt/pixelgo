package ratelimit

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type Decision struct {
	Allowed    bool
	Limit      int
	Remaining  int
	ResetAfter time.Duration
}

type Limiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (Decision, error)
}

type memoryWindow struct {
	count     int
	expiresAt time.Time
}

type MemoryLimiter struct {
	mu      sync.Mutex
	windows map[string]memoryWindow
}

func NewMemoryLimiter() *MemoryLimiter {
	return &MemoryLimiter{windows: make(map[string]memoryWindow)}
}

func (l *MemoryLimiter) Allow(
	_ context.Context,
	key string,
	limit int,
	window time.Duration,
) (Decision, error) {
	now := time.Now()

	l.mu.Lock()
	entry, ok := l.windows[key]
	if !ok || !entry.expiresAt.After(now) {
		entry = memoryWindow{expiresAt: now.Add(window)}
	}
	entry.count++
	l.windows[key] = entry
	l.mu.Unlock()

	remaining := limit - entry.count
	if remaining < 0 {
		remaining = 0
	}

	return Decision{
		Allowed:    entry.count <= limit,
		Limit:      limit,
		Remaining:  remaining,
		ResetAfter: time.Until(entry.expiresAt),
	}, nil
}

type RedisLimiter struct {
	client *redis.Client
	prefix string
}

func NewRedisLimiter(client *redis.Client) *RedisLimiter {
	return &RedisLimiter{
		client: client,
		prefix: "pixelgo:ratelimit:",
	}
}

var fixedWindowScript = redis.NewScript(`
local count = redis.call("INCR", KEYS[1])
if count == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
local ttl = redis.call("PTTL", KEYS[1])
return {count, ttl}
`)

func (l *RedisLimiter) Allow(
	ctx context.Context,
	key string,
	limit int,
	window time.Duration,
) (Decision, error) {
	values, err := fixedWindowScript.Run(
		ctx,
		l.client,
		[]string{l.prefix + key},
		window.Milliseconds(),
	).Int64Slice()
	if err != nil {
		return Decision{}, err
	}

	count := int(values[0])
	resetAfter := time.Duration(values[1]) * time.Millisecond
	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}

	return Decision{
		Allowed:    count <= limit,
		Limit:      limit,
		Remaining:  remaining,
		ResetAfter: resetAfter,
	}, nil
}
