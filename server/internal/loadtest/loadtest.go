// Package loadtest provides a bounded, repeatable HTTP smoke benchmark.
package loadtest

import (
    "context"
    "errors"
    "math"
    "net/http"
    "sort"
    "sync"
    "time"
)

type Report struct {
    Requests int `json:"requests"`
    Successful int `json:"successful"`
    Failed int `json:"failed"`
    TotalSeconds float64 `json:"total_seconds"`
    RequestsPerSecond float64 `json:"requests_per_second"`
    P50LatencyMs float64 `json:"p50_latency_ms"`
    P95LatencyMs float64 `json:"p95_latency_ms"`
    P99LatencyMs float64 `json:"p99_latency_ms"`
}

func Run(ctx context.Context, client *http.Client, target string, requests, concurrency int) (Report, error) {
    if requests < 1 || requests > 100000 || concurrency < 1 || concurrency > 256 || target == "" {
        return Report{}, errors.New("invalid load-test arguments")
    }
    jobs := make(chan struct{})
    latencies := make(chan float64, requests)
    results := make(chan bool, requests)
    var workers sync.WaitGroup
    start := time.Now()
    for i := 0; i < concurrency; i++ {
        workers.Add(1)
        go func() {
            defer workers.Done()
            for range jobs {
                begin := time.Now()
                request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
                ok := false
                if err == nil {
                    response, callErr := client.Do(request)
                    if callErr == nil {
                        ok = response.StatusCode >= 200 && response.StatusCode < 300
                        response.Body.Close()
                    }
                }
                latencies <- float64(time.Since(begin).Microseconds()) / 1000.0
                results <- ok
            }
        }()
    }
    for i := 0; i < requests; i++ { jobs <- struct{}{} }
    close(jobs)
    workers.Wait()
    close(latencies)
    close(results)
    report := Report{Requests: requests, TotalSeconds: time.Since(start).Seconds()}
    series := make([]float64, 0, requests)
    for value := range latencies { series = append(series, value) }
    for ok := range results {
        if ok { report.Successful++ } else { report.Failed++ }
    }
    sort.Float64s(series)
    quantile := func(p float64) float64 { return series[int(math.Ceil(float64(len(series))*p))-1] }
    report.P50LatencyMs = quantile(.50)
    report.P95LatencyMs = quantile(.95)
    report.P99LatencyMs = quantile(.99)
    report.RequestsPerSecond = float64(requests) / report.TotalSeconds
    return report, nil
}
