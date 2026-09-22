package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"sync"
	"time"
)

const maxIdempotentBodyBytes = 1 << 20

type idempotencyEntry struct {
	fingerprint string
	status      int
	header      http.Header
	body        []byte
	expiresAt   time.Time
}

type idempotencyStore struct {
	mu      sync.Mutex
	entries map[string]idempotencyEntry
	ttl     time.Duration
}

func newIdempotencyStore(ttl time.Duration) *idempotencyStore {
	return &idempotencyStore{
		entries: make(map[string]idempotencyEntry),
		ttl:     ttl,
	}
}

func withIdempotency(next http.Handler, store *idempotencyStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" || r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}
		if len(key) > 128 {
			writeError(w, http.StatusBadRequest, "invalid_idempotency_key", "idempotency key exceeds 128 characters")
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, maxIdempotentBodyBytes+1))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "could not read request body")
			return
		}
		if len(body) > maxIdempotentBodyBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds 1 MiB")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		fingerprint := requestFingerprint(r, body)
		now := time.Now()

		store.mu.Lock()
		store.pruneExpiredLocked(now)
		if existing, ok := store.entries[key]; ok {
			store.mu.Unlock()
			if existing.fingerprint != fingerprint {
				writeError(w, http.StatusConflict, "idempotency_key_reused", "idempotency key was already used with a different request")
				return
			}
			copyHeader(w.Header(), existing.header)
			w.Header().Set("Idempotency-Replayed", "true")
			w.WriteHeader(existing.status)
			_, _ = w.Write(existing.body)
			return
		}
		store.mu.Unlock()

		recorder := newBufferedResponseWriter()
		next.ServeHTTP(recorder, r)

		copyHeader(w.Header(), recorder.header)
		w.WriteHeader(recorder.status)
		_, _ = w.Write(recorder.body.Bytes())

		if recorder.status >= 500 {
			return
		}

		store.mu.Lock()
		store.entries[key] = idempotencyEntry{
			fingerprint: fingerprint,
			status:      recorder.status,
			header:      recorder.header.Clone(),
			body:        append([]byte(nil), recorder.body.Bytes()...),
			expiresAt:   now.Add(store.ttl),
		}
		store.mu.Unlock()
	})
}

func requestFingerprint(r *http.Request, body []byte) string {
	sum := sha256.New()
	_, _ = sum.Write([]byte(r.Method))
	_, _ = sum.Write([]byte{0})
	_, _ = sum.Write([]byte(r.URL.Path))
	_, _ = sum.Write([]byte{0})
	_, _ = sum.Write(body)
	return hex.EncodeToString(sum.Sum(nil))
}

func (s *idempotencyStore) pruneExpiredLocked(now time.Time) {
	for key, entry := range s.entries {
		if !entry.expiresAt.After(now) {
			delete(s.entries, key)
		}
	}
}

type bufferedResponseWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newBufferedResponseWriter() *bufferedResponseWriter {
	return &bufferedResponseWriter{
		header: make(http.Header),
		status: http.StatusOK,
	}
}

func (w *bufferedResponseWriter) Header() http.Header {
	return w.header
}

func (w *bufferedResponseWriter) WriteHeader(status int) {
	if w.status != http.StatusOK || w.body.Len() > 0 {
		return
	}
	w.status = status
}

func (w *bufferedResponseWriter) Write(p []byte) (int, error) {
	return w.body.Write(p)
}

func copyHeader(dst, src http.Header) {
	for key := range dst {
		dst.Del(key)
	}
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
