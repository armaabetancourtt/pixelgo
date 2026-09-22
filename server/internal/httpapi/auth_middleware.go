package httpapi

import (
	"net/http"
	"strings"

	"github.com/armaabetancourtt/pixelgo/server/internal/auth"
)

func withAuthentication(next http.Handler, service *auth.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicWithoutBearer(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		header := strings.TrimSpace(r.Header.Get("Authorization"))
		const prefix = "Bearer "
		if !strings.HasPrefix(header, prefix) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "bearer access token required")
			return
		}

		raw := strings.TrimSpace(strings.TrimPrefix(header, prefix))
		userID, err := service.ValidateAccess(raw)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "access token is invalid or expired")
			return
		}

		next.ServeHTTP(w, r.WithContext(auth.ContextWithUserID(r.Context(), userID)))
	})
}

func publicWithoutBearer(path string) bool {
	switch path {
	case "/health", "/metrics", "/v1/auth/register", "/v1/auth/login", "/v1/auth/refresh":
		return true
	}
	return strings.HasPrefix(path, "/dev-upload/") ||
		strings.HasPrefix(path, "/dev-download/")
}
