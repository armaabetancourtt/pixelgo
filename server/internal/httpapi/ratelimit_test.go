package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/armaabetancourtt/pixelgo/server/internal/ratelimit"
)

func TestRateLimitReturns429AndHeaders(t *testing.T) {
	limiter := ratelimit.NewMemoryLimiter()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := withRateLimit(next, limiter)

	var last *httptest.ResponseRecorder
	for i := 0; i <= readRequestLimit; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/devices", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		last = httptest.NewRecorder()
		handler.ServeHTTP(last, req)
	}

	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", last.Code)
	}
	if last.Header().Get("RateLimit-Limit") == "" {
		t.Fatal("expected RateLimit-Limit header")
	}
	if last.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
}
