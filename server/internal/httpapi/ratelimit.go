package httpapi

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/armaabetancourtt/pixelgo/server/internal/ratelimit"
)

const (
	readRequestLimit     = 240
	mutationRequestLimit = 120
	rateLimitWindow      = time.Minute
)

func withRateLimit(next http.Handler, limiter ratelimit.Limiter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/") {
			next.ServeHTTP(w, r)
			return
		}

		limit := readRequestLimit
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			limit = mutationRequestLimit
		}

		key := clientRateLimitKey(r)
		decision, err := limiter.Allow(r.Context(), key, limit, rateLimitWindow)
		if err != nil {
			// Rate limiting is an abuse-control layer, not a consistency
			// boundary. Fail open so a Redis outage does not take the API down.
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("RateLimit-Limit", strconv.Itoa(decision.Limit))
		w.Header().Set("RateLimit-Remaining", strconv.Itoa(decision.Remaining))

		if !decision.Allowed {
			retryAfter := int(decision.ResetAfter.Round(time.Second).Seconds())
			if retryAfter < 1 {
				retryAfter = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			writeError(w, http.StatusTooManyRequests, "rate_limited", "too many requests")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func clientRateLimitKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host == "" {
		host = r.RemoteAddr
	}
	if host == "" {
		host = "unknown"
	}

	scope := "read"
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		scope = "mutation"
	}
	return scope + ":" + host
}
