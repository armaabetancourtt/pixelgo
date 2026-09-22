package presence

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/armaabetancourtt/pixelgo/server/internal/platform/redisdb"
)

func TestMemoryPresenceExpires(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	if err := store.Online(ctx, "dev_1", 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	online, err := store.IsOnline(ctx, "dev_1")
	if err != nil || !online {
		t.Fatalf("expected online, got online=%v err=%v", online, err)
	}

	time.Sleep(30 * time.Millisecond)
	online, err = store.IsOnline(ctx, "dev_1")
	if err != nil {
		t.Fatal(err)
	}
	if online {
		t.Fatal("expected presence TTL to expire")
	}
}

func TestRedisPresenceLifecycle(t *testing.T) {
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

	store := NewRedisStore(client)
	deviceID := "presence-integration-test"

	if err := store.Online(ctx, deviceID, time.Minute); err != nil {
		t.Fatal(err)
	}
	online, err := store.IsOnline(ctx, deviceID)
	if err != nil || !online {
		t.Fatalf("expected online, got online=%v err=%v", online, err)
	}

	if err := store.Offline(ctx, deviceID); err != nil {
		t.Fatal(err)
	}
	online, err = store.IsOnline(ctx, deviceID)
	if err != nil {
		t.Fatal(err)
	}
	if online {
		t.Fatal("expected offline after explicit disconnect")
	}
}
