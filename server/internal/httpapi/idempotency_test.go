package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIdempotencyReplaysIdenticalRequest(t *testing.T) {
	var calls int32
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		writeJSON(w, http.StatusCreated, map[string]string{"id": "tr_stable"})
	})
	handler := withJSON(withIdempotency(next, newIdempotencyStore(time.Hour)))

	first := idempotentRequest(handler, "same-key", []byte(`{"kind":"photo"}`))
	second := idempotentRequest(handler, "same-key", []byte(`{"kind":"photo"}`))

	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Fatalf("expected 201/201, got %d/%d", first.Code, second.Code)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected handler to execute once, got %d", got)
	}
	if second.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatal("expected replay response header")
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("expected identical response bodies, got %q and %q", first.Body.String(), second.Body.String())
	}
}

func TestIdempotencyCoalescesConcurrentRetries(t *testing.T) {
	var calls int32
	started := make(chan struct{})
	release := make(chan struct{})

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			close(started)
		}
		<-release
		writeJSON(w, http.StatusCreated, map[string]string{"id": "tr_one"})
	})
	handler := withJSON(withIdempotency(next, newIdempotencyStore(time.Hour)))

	var first, second *httptest.ResponseRecorder
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		first = idempotentRequest(handler, "parallel-key", []byte(`{"kind":"photo"}`))
	}()

	<-started

	go func() {
		defer wg.Done()
		second = idempotentRequest(handler, "parallel-key", []byte(`{"kind":"photo"}`))
	}()

	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected one mutation execution, got %d", got)
	}
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Fatalf("expected both requests to receive 201, got %d/%d", first.Code, second.Code)
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("expected identical responses, got %q and %q", first.Body.String(), second.Body.String())
	}
}

func TestIdempotencyRejectsKeyReuseWithDifferentPayload(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusCreated, map[string]string{"ok": "true"})
	})
	handler := withJSON(withIdempotency(next, newIdempotencyStore(time.Hour)))

	_ = idempotentRequest(handler, "same-key", []byte(`{"kind":"photo"}`))
	conflict := idempotentRequest(handler, "same-key", []byte(`{"kind":"file"}`))

	if conflict.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", conflict.Code)
	}
	if !bytes.Contains(conflict.Body.Bytes(), []byte("idempotency_key_reused")) {
		t.Fatalf("expected idempotency error, got %s", conflict.Body.String())
	}
}

func TestIdempotencyDoesNotCacheServerErrors(t *testing.T) {
	var calls int32
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&calls, 1)
		if call == 1 {
			writeError(w, http.StatusServiceUnavailable, "temporary", "retry")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": "tr_recovered"})
	})
	handler := withJSON(withIdempotency(next, newIdempotencyStore(time.Hour)))

	first := idempotentRequest(handler, "retry-key", []byte(`{"kind":"photo"}`))
	second := idempotentRequest(handler, "retry-key", []byte(`{"kind":"photo"}`))

	if first.Code != http.StatusServiceUnavailable || second.Code != http.StatusCreated {
		t.Fatalf("expected 503 then 201, got %d then %d", first.Code, second.Code)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected handler to execute twice, got %d", got)
	}
}

func idempotentRequest(handler http.Handler, key string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/transfers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
