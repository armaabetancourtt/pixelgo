package ratelimit

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/armaabetancourtt/pixelgo/server/internal/platform/redisdb"
)

func TestMemoryLimiterRejectsAfterLimit(t *testing.T) {
	limiter := NewMemoryLimiter()
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		decision, err := limiter.Allow(ctx, "client", 2, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if !decision.Allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	decision, err := limiter.Allow(ctx, "client", 2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed {
		t.Fatal("third request should be rate limited")
	}
}

func TestRedisLimiterSharesBudgetAcrossInstances(t *testing.T) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		t.Skip("REDIS_URL is not configured")
	}

	ctx := context.Background()
	client, err := redisdb.Open(ctx, redisURL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	first := NewRedisLimiter(client)
	second := NewRedisLimiter(client)
	key := "integration-" + time.Now().UTC().Format("150405.000000000")

	one, err := first.Allow(ctx, key, 2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	two, err := second.Allow(ctx, key, 2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	three, err := first.Allow(ctx, key, 2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	if !one.Allowed || !two.Allowed || three.Allowed {
		t.Fatalf("expected shared 2-request budget, got %+v %+v %+v", one, two, three)
	}
	if two.Remaining != 0 {
		t.Fatalf("expected shared remaining=0 after two requests, got %d", two.Remaining)
	}
}
