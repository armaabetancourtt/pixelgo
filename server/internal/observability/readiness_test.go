package observability

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadinessHandlerReportsReady(t *testing.T) {
	handler := ReadinessHandler(
		[]Check{
			{
				Name: "postgres",
				Run: func(context.Context) error { return nil },
			},
			{
				Name: "redis",
				Run: func(context.Context) error { return nil },
			},
		},
		time.Second,
	)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/ready", nil),
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "\"status\":\"ready\"") {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestReadinessHandlerFailsClosedWithoutLeakingError(t *testing.T) {
	handler := ReadinessHandler(
		[]Check{
			{
				Name: "object_storage",
				Run: func(context.Context) error {
					return errors.New("secret endpoint details")
				},
			},
		},
		time.Second,
	)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/ready", nil),
	)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "\"status\":\"not_ready\"") {
		t.Fatalf("unexpected body: %s", body)
	}
	if strings.Contains(body, "secret endpoint details") {
		t.Fatal("readiness response leaked dependency error details")
	}
}
