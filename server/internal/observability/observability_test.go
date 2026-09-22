package observability

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddlewareAddsRequestIDAndMetrics(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	metrics := NewMetrics()

	handler := Middleware(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("created"))
		}),
		logger,
		metrics,
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/transfers",
		nil,
	)
	request.Header.Set("Authorization", "Bearer should-never-be-logged")
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", recorder.Code)
	}
	if recorder.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected X-Request-ID")
	}

	metricsRecorder := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(
		metricsRecorder,
		httptest.NewRequest(http.MethodGet, "/metrics", nil),
	)
	body := metricsRecorder.Body.String()
	expected := "pixelgo_http_requests_total{method=\"POST\",route=\"/v1/transfers\",status=\"201\"} 1"
	if !strings.Contains(body, expected) {
		t.Fatalf("missing request metric: %s", body)
	}

	line := strings.TrimSpace(logs.String())
	var event map[string]any
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatalf("parse structured log: %v", err)
	}
	if event["method"] != "POST" || event["status"] != float64(201) {
		t.Fatalf("unexpected log event: %#v", event)
	}
	if strings.Contains(line, "should-never-be-logged") {
		t.Fatal("authorization credential leaked into structured log")
	}
}

func TestNormalizedRouteAvoidsResourceIDCardinality(t *testing.T) {
	cases := map[string]string{
		"/v1/devices/dev_abc":             "/v1/devices/{deviceId}",
		"/v1/presence/dev_abc":            "/v1/presence/{deviceId}",
		"/v1/transfers/tr_abc":             "/v1/transfers/{transferId}",
		"/v1/transfers/tr_abc/uploaded":    "/v1/transfers/{transferId}/uploaded",
		"/v1/transfers/tr_abc/complete":    "/v1/transfers/{transferId}/complete",
		"/dev-upload/tr_abc":               "/dev-upload/{transferId}",
		"/dev-download/tr_abc":             "/dev-download/{transferId}",
		"/some/random/high/cardinality/id": "unmatched",
	}

	for input, expected := range cases {
		if got := normalizedRoute(input); got != expected {
			t.Fatalf("%s: expected %s, got %s", input, expected, got)
		}
	}
}
