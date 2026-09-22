package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/armaabetancourtt/pixelgo/server/internal/platform/redisdb"
	"github.com/redis/go-redis/v9"
)

func TestRedisIdempotencyReplaysAcrossHandlers(t *testing.T) {
	client := testRedisClient(t)

	var calls int32
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		writeJSON(w, http.StatusCreated, map[string]string{"id": "tr_shared"})
	})

	firstHandler := withRedisIdempotency(next, client, time.Hour)
	secondHandler := withRedisIdempotency(next, client, time.Hour)
	key := "redis-replay-" + time.Now().UTC().Format("150405.000000000")

	first := redisIdempotentRequest(firstHandler, key, []byte(`{"kind":"photo"}`))
	second := redisIdempotentRequest(secondHandler, key, []byte(`{"kind":"photo"}`))

	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Fatalf("expected 201/201, got %d/%d", first.Code, second.Code)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected one mutation across handlers, got %d", got)
	}
	if second.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatal("expected replay header from second handler")
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("expected identical bodies, got %q and %q", first.Body.String(), second.Body.String())
	}
}

func TestRedisIdempotencyCoalescesConcurrentReplicaRetries(t *testing.T) {
	client := testRedisClient(t)

	var calls int32
	started := make(chan struct{})
	release := make(chan struct{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			close(started)
		}
		<-release
		writeJSON(w, http.StatusCreated, map[string]string{"id": "tr_distributed"})
	})

	firstHandler := withRedisIdempotency(next, client, time.Hour)
	secondHandler := withRedisIdempotency(next, client, time.Hour)
	key := "redis-concurrent-" + time.Now().UTC().Format("150405.000000000")

	var first, second *httptest.ResponseRecorder
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		first = redisIdempotentRequest(firstHandler, key, []byte(`{"kind":"file"}`))
	}()

	<-started

	go func() {
		defer wg.Done()
		second = redisIdempotentRequest(secondHandler, key, []byte(`{"kind":"file"}`))
	}()

	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected one distributed mutation, got %d", got)
	}
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Fatalf("expected 201/201, got %d/%d", first.Code, second.Code)
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("expected identical bodies, got %q and %q", first.Body.String(), second.Body.String())
	}
}

func TestRedisIdempotencyRejectsCrossReplicaKeyReuse(t *testing.T) {
	client := testRedisClient(t)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusCreated, map[string]string{"ok": "true"})
	})

	firstHandler := withRedisIdempotency(next, client, time.Hour)
	secondHandler := withRedisIdempotency(next, client, time.Hour)
	key := "redis-conflict-" + time.Now().UTC().Format("150405.000000000")

	_ = redisIdempotentRequest(firstHandler, key, []byte(`{"kind":"photo"}`))
	conflict := redisIdempotentRequest(secondHandler, key, []byte(`{"kind":"text"}`))

	if conflict.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", conflict.Code)
	}
	if !bytes.Contains(conflict.Body.Bytes(), []byte("idempotency_key_reused")) {
		t.Fatalf("expected idempotency conflict body, got %s", conflict.Body.String())
	}
}

func testRedisClient(t *testing.T) *redis.Client {
	return openRedisForIdempotencyTest(t)
}

func redisIdempotentRequest(handler http.Handler, key string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/transfers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func openRedisForIdempotencyTest(t *testing.T) *redis.Client {
	t.Helper()
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		t.Skip("REDIS_URL is not configured")
	}

	client, err := redisdb.Open(context.Background(), redisURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}
