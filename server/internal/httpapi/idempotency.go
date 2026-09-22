package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

const maxIdempotentBodyBytes = 1 << 20

var errRequestBodyTooLarge = errors.New("request body too large")

type idempotencyEntry struct {
	fingerprint string
	ready       chan struct{}
	status      int
	header      http.Header
	body        []byte
	expiresAt   time.Time
}

type idempotencyStore struct {
	mu      sync.Mutex
	entries map[string]*idempotencyEntry
	ttl     time.Duration
}

func newIdempotencyStore(ttl time.Duration) *idempotencyStore {
	return &idempotencyStore{
		entries: make(map[string]*idempotencyEntry),
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

		body, err := readRequestBody(r)
		if err != nil {
			if errors.Is(err, errRequestBodyTooLarge) {
				writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds 1 MiB")
			} else {
				writeError(w, http.StatusBadRequest, "invalid_request", "could not read request body")
			}
			return
		}

		fingerprint := requestFingerprint(r, body)
		now := time.Now()

		store.mu.Lock()
		store.pruneExpiredLocked(now)
		if existing, ok := store.entries[key]; ok {
			if existing.fingerprint != fingerprint {
				store.mu.Unlock()
				writeError(w, http.StatusConflict, "idempotency_key_reused", "idempotency key was already used with a different request")
				return
			}
			ready := existing.ready
			store.mu.Unlock()

			<-ready
			copyHeader(w.Header(), existing.header)
			w.Header().Set("Idempotency-Replayed", "true")
			w.WriteHeader(existing.status)
			_, _ = w.Write(existing.body)
			return
		}

		entry := &idempotencyEntry{
			fingerprint: fingerprint,
			ready:       make(chan struct{}),
		}
		store.entries[key] = entry
		store.mu.Unlock()

		recorder := newBufferedResponseWriter()
		next.ServeHTTP(recorder, r)

		store.mu.Lock()
		entry.status = recorder.status
		entry.header = recorder.header.Clone()
		entry.body = append([]byte(nil), recorder.body.Bytes()...)
		entry.expiresAt = now.Add(store.ttl)
		if recorder.status >= http.StatusInternalServerError {
			delete(store.entries, key)
		}
		store.mu.Unlock()

		close(entry.ready)

		copyHeader(w.Header(), recorder.header)
		w.WriteHeader(recorder.status)
		_, _ = w.Write(recorder.body.Bytes())
	})
}

func readRequestBody(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxIdempotentBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxIdempotentBodyBytes {
		return nil, errRequestBodyTooLarge
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
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
		if !entry.expiresAt.IsZero() && !entry.expiresAt.After(now) {
			delete(s.entries, key)
		}
	}
}

type bufferedResponseWriter struct {
	header      http.Header
	status      int
	wroteHeader bool
	body        bytes.Buffer
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
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
}

func (w *bufferedResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
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
