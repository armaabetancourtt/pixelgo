package presence

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestMemoryStoreKeepsDeviceOnlineWhileAnyLeaseLives(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	if err := store.Touch(ctx, "dev_1", "lease_a", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := store.Touch(ctx, "dev_1", "lease_b", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(ctx, "dev_1", "lease_a"); err != nil {
		t.Fatal(err)
	}

	state, err := store.Online(ctx, []string{"dev_1"})
	if err != nil {
		t.Fatal(err)
	}
	if !state["dev_1"] {
		t.Fatal("expected device to remain online while second lease exists")
	}

	if err := store.Remove(ctx, "dev_1", "lease_b"); err != nil {
		t.Fatal(err)
	}
	state, err = store.Online(ctx, []string{"dev_1"})
	if err != nil {
		t.Fatal(err)
	}
	if state["dev_1"] {
		t.Fatal("expected device to be offline after final lease is removed")
	}
}

func TestMemoryStoreExpiresStaleLease(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	if err := store.Touch(ctx, "dev_stale", "lease_stale", 5*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(15 * time.Millisecond)

	state, err := store.Online(ctx, []string{"dev_stale"})
	if err != nil {
		t.Fatal(err)
	}
	if state["dev_stale"] {
		t.Fatal("expected stale lease to expire")
	}
}

func TestRedisStoreLeaseIntegration(t *testing.T) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		t.Skip("REDIS_URL not configured")
	}

	ctx := context.Background()
	store, err := NewRedisStore(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	if err := store.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	deviceID := fmt.Sprintf("dev_test_%d", time.Now().UnixNano())
	if err := store.Touch(ctx, deviceID, "lease_a", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := store.Touch(ctx, deviceID, "lease_b", time.Minute); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = store.Remove(context.Background(), deviceID, "lease_a")
		_ = store.Remove(context.Background(), deviceID, "lease_b")
	}()

	state, err := store.Online(ctx, []string{deviceID})
	if err != nil {
		t.Fatal(err)
	}
	if !state[deviceID] {
		t.Fatal("expected redis-backed device to be online")
	}

	if err := store.Remove(ctx, deviceID, "lease_a"); err != nil {
		t.Fatal(err)
	}
	state, err = store.Online(ctx, []string{deviceID})
	if err != nil {
		t.Fatal(err)
	}
	if !state[deviceID] {
		t.Fatal("expected second redis lease to keep device online")
	}

	if err := store.Remove(ctx, deviceID, "lease_b"); err != nil {
		t.Fatal(err)
	}
	state, err = store.Online(ctx, []string{deviceID})
	if err != nil {
		t.Fatal(err)
	}
	if state[deviceID] {
		t.Fatal("expected redis-backed device to be offline after final lease")
	}
}
