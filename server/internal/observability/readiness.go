package observability

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type Check struct {
	Name string
	Run  func(context.Context) error
}

func ReadinessHandler(
	checks []Check,
	timeout time.Duration,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		status := "ready"
		code := http.StatusOK
		results := make(map[string]string, len(checks))

		for _, check := range checks {
			if ctx.Err() != nil {
				results[check.Name] = "unavailable"
				status = "not_ready"
				code = http.StatusServiceUnavailable
				continue
			}

			if err := check.Run(ctx); err != nil {
				results[check.Name] = "unavailable"
				status = "not_ready"
				code = http.StatusServiceUnavailable
				continue
			}
			results[check.Name] = "ok"
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": status,
			"checks": results,
		})
	})
}
