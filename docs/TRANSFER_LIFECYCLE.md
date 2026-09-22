# Transfer lifecycle

A transfer is metadata plus a payload reference, not the payload itself.

## States

- `created`: durable intent exists.
- `uploading`: sender has authorization to upload.
- `ready`: upload was confirmed; destination can download.
- `downloading`: optional destination acknowledgement for large payloads.
- `completed`: destination verified integrity and acknowledged delivery.
- `failed`: terminal failure requiring explicit retry/new transfer.

Current server foundation creates transfers directly in `uploading`.

## Invariants

- A transfer cannot complete before it is ready.
- The destination must belong to the same authorized account.
- The object key is never chosen directly by an untrusted client.
- Signed URLs are short lived.
- SHA-256 is checked after download.
- Mutation endpoints accept idempotency keys.
- Event duplication must be harmless.
- Event loss must be recoverable by querying durable state.

## Why WebSocket + push?

WebSocket is the fast path while the app is connected. Push is a wake-up path when the OS suspended or killed the app.

Push should contain only enough metadata to tell the client what to query. It should not carry the file payload.

## Retry model

A failed upload may request a fresh signed URL for the same transfer while the transfer remains uploadable. A duplicate `uploaded` or `complete` request with the same idempotency key must not create a second logical operation.

Those idempotency semantics are part of the next persistence milestone.
