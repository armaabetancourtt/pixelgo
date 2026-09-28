// CLI for a bounded health/readiness HTTP load smoke test (not user-journey SLO evidence).
package main

import (
    "context"
    "encoding/json"
    "flag"
    "fmt"
    "net/http"
    "net/url"
    "os"
    "time"

    "github.com/armaabetancourtt/pixelgo/server/internal/loadtest"
)

func main() {
    target := flag.String("url", "http://127.0.0.1:8080/health", "exact health/readiness URL")
    requests := flag.Int("requests", 200, "number of requests (max 100000)")
    concurrency := flag.Int("concurrency", 10, "workers (max 256)")
    timeout := flag.Duration("timeout", 5*time.Second, "per-request timeout")
    flag.Parse()
    parsed, err := url.Parse(*target)
    if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
        fmt.Fprintln(os.Stderr, "invalid HTTP(S) target")
        os.Exit(2)
    }
    client := &http.Client{Timeout: *timeout}
    report, err := loadtest.Run(context.Background(), client, *target, *requests, *concurrency)
    if err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(2)
    }
    output, err := json.MarshalIndent(report, "", "  ")
    if err != nil { panic(err) }
    fmt.Println(string(output))
    if report.Failed > 0 { os.Exit(1) }
}
