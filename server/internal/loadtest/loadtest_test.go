package loadtest

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestRunSuccess(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))
    defer server.Close()
    report, err := Run(context.Background(), server.Client(), server.URL, 31, 4)
    if err != nil { t.Fatal(err) }
    if report.Requests != 31 || report.Successful != 31 || report.Failed != 0 ||
       report.P95LatencyMs < 0 || report.RequestsPerSecond <= 0 {
        t.Fatalf("unexpected report: %+v", report)
    }
}

func TestRunCountsFailures(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.WriteHeader(http.StatusServiceUnavailable)
    }))
    defer server.Close()
    report, err := Run(context.Background(), server.Client(), server.URL, 10, 2)
    if err != nil || report.Failed != 10 { t.Fatalf("%+v %v", report, err) }
}

func TestRunRejectsUnboundedLoad(t *testing.T) {
    _, err := Run(context.Background(), http.DefaultClient, "http://localhost", 100001, 1)
    if err == nil { t.Fatal("expected bounds validation") }
}
