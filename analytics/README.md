# Pixel Go: operational transfer event analytics (experimental)

A separately executable **ETL metadata sink** demonstrating idempotent event processing, structured schema, historical SQL aggregates and quality gates. The production Go transfer system is **not modified or connected**.

Input JSONL keys: `event_id`, `transfer_id`, `event_time` (timezone-aware ISO-8601), `status` (created/queued/processing/completed/failed/cancelled).
```bash
python3 -m unittest discover -s analytics -p 'test_*.py'
python3 -m analytics.pipeline --input transfers.jsonl --db local.sqlite
```

No tokens, account identifiers or file content belong in this feed. Events must come from a permissioned producer with appropriate retention and access rules. Event IDs ensure idempotency on retry.

Proposed production data architecture: PostgreSQL outbox -> Kafka -> object-storage lake -> Spark/stream analytics -> dbt warehouse mart -> dashboard. The real system would also require checkpointing, replay tests, dead-letter queues, consistency semantics, IAM, metrics, SLOs and incident response. These services are **not currently deployed** as part of this change.
