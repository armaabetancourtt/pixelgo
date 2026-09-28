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
    "strings"
    "time"

    "github.com/armaabetancourtt/pixelgo/server/internal/loadtest"
)

func main() {
    target := flag.String("url", "http://127.0.0.1:8080/health", "exact health/readiness URL")
    requests := flag.Int("requests", 200, "number of requests (max 100000)")
    concurrency := flag.Int("concurrency", 10, "workers (max 256)")
    timeout := flag.Duration("timeout", 5*time.Second, "per-request timeout")
    allowRemote := flag.Bool("allow-remote", false, "explicitly authorize requests to a non-loopback staging host")
    flag.Parse()
    if err := validateTarget(*target, *allowRemote); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(2)
    }
    if *timeout <= 0 || *timeout > 60*time.Second {
        fmt.Fprintln(os.Stderr, "timeout must be between 1ns and 60s")
        os.Exit(2)
    }
    client := &http.Client{
        Timeout: *timeout,
        // Never silently follow a local endpoint's redirect into an external target.
        CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
            return http.ErrUseLastResponse
        },
    }
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

func validateTarget(raw string, allowRemote bool) error {
    parsed, err := url.Parse(raw)
    if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
        parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
        return fmt.Errorf("invalid HTTP(S) target")
    }
    host := strings.ToLower(parsed.Hostname())
    if !allowRemote && host != "127.0.0.1" && host != "::1" && host != "localhost" {
        return fmt.Errorf("remote load testing requires explicit -allow-remote on an authorized host")
    }
    return nil
}
