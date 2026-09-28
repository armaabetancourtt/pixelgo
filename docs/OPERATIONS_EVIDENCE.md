# Reproducible operations evidence

PIXEL GO already implements readiness/health, request metrics and deployment gating. This addition supplies a **bounded and repeatable smoke-load command** that records real results rather than publishing made-up latency.

\`\`\`bash
cd server
go test ./internal/loadtest/... ./internal/observability/...
# With the local API launched by the documented compose stack:
go run ./cmd/loadtest -url http://127.0.0.1:8080/health -requests 200 -concurrency 10
go run ./cmd/loadtest -url http://127.0.0.1:8080/ready -requests 200 -concurrency 10
\`\`\`

The CLI emits requests, successful/failed responses, requests/sec and p50/p95/p99 latency (milliseconds), exits nonzero on failures and uses capped request/concurrency parameters. Execute on an authorized staging environment only; never load test public production without approval. Save command, commit SHA, runtime, environment, date, hardware, load level and JSON result with each report. Compare against Prometheus \`pixelgo_http_request_duration_seconds\`, \`pixelgo_http_requests_total\` and in-flight gauges to investigate discrepancies. The test intentionally targets lightweight health/readiness endpoints: **it is not evidence of transfer upload throughput or a production SLO**.

For a failure-recovery exercise, in local development stop Redis or PostgreSQL separately, observe \`/ready\`, restart the dependency, and verify recovery. Record timestamps, retry behavior and idempotent transfer outcomes before claiming RTO/RPO. Use \`docs/DEPLOYMENT.md\` for image-promotion/rollback gates. Full authenticated transfer-load workloads and distributed fault-injection remain planned.
