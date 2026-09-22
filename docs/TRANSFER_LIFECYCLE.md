# Transfer lifecycle

A PIXEL GO transfer is durable metadata plus a payload-storage boundary. The application server owns authorization and state; production payload bytes should live in object storage.

## States

- `created`: durable intent exists.
- `uploading`: sender has authorization to upload.
- `ready`: a valid payload exists and the destination may download.
- `downloading`: optional destination acknowledgement for larger transfers.
- `completed`: destination verified the payload and acknowledged delivery.
- `failed`: terminal failure requiring explicit retry or replacement.

The current foundation creates a transfer directly in `uploading`.

## Current executable flow

```text
POST /v1/transfers
      ↓
signed upload URL
      ↓
PUT actual payload bytes
      ↓
validate exact byte count
      ↓
validate SHA-256
      ↓
POST /uploaded
      ↓
transfer.ready
      ↓
signed download URL
      ↓
GET exact payload bytes
      ↓
destination validates SHA-256
      ↓
POST /complete
      ↓
transfer.completed
```

The CI E2E executes this path with real bytes.

## Signed URLs

The development adapter signs URLs with HMAC-SHA256 over:

```text
action
transfer-id
expiry
```

Upload and download signatures are therefore not interchangeable. A signature is scoped to one transfer and expires.

The in-memory development adapter exists to make local development and CI deterministic. Production should replace it with an S3-compatible adapter that issues short-lived provider-signed URLs so large payloads bypass application-server memory.

## Integrity invariants

The current development upload route refuses to accept a payload unless:

- the transfer exists;
- the transfer is still in `uploading`;
- the byte count exactly equals declared `sizeBytes`;
- SHA-256 exactly equals the checksum declared when the transfer was created.

`POST /uploaded` additionally refuses to move the transfer to `ready` unless a payload actually exists.

These checks prevent metadata from claiming that a file is ready when no validated bytes were received.

## Retry model

Mobile clients can lose the response to a successful mutation. PIXEL GO implements `Idempotency-Key` on POST mutations to make retries safe.

Current semantics:

- same key + same request fingerprint → original response replayed;
- same key + different request → conflict;
- concurrent identical retries wait on one executing mutation and then replay it;
- server failures are not persisted as successful idempotency records.

The current idempotency registry is intentionally in-memory. Production must move it to shared durable/ephemeral infrastructure such as Redis or PostgreSQL so guarantees survive process restarts and multiple API replicas.

## Realtime delivery

Once an upload is confirmed, the transfer service publishes `transfer.ready`.

WebSocket is the connected-device fast path. Push is intended as a wake-up path when an OS has suspended the application.

Push should carry enough metadata to tell the client what to query; it should not contain the transferred file.

## Recovery principle

Realtime events are acceleration, not durable truth.

If an event is duplicated, the client should remain correct. If an event is lost, the client must be able to recover by querying transfer state from the API.

That distinction is important because mobile network delivery is never perfectly reliable.
