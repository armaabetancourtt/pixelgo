# Security model

PIXEL GO handles personal files and device credentials, so security boundaries are product requirements.

## Credentials

- Access tokens should be short lived.
- Refresh tokens should rotate on use and belong to a token family.
- iOS stores credential material in Keychain.
- Android protects credential material with Android Keystore.
- Tokens must never be logged.

## Files

- Clients upload/download with short-lived signed URLs.
- Object keys are opaque server-generated values.
- Payload size and content type are validated.
- SHA-256 verifies transport integrity after download.
- Objects expire and orphaned uploads are cleaned by workers.
- The API does not expose a public bucket.

## Network

- TLS is required outside local development.
- Local cleartext networking exists only for emulator/simulator development.
- WebSocket connections currently identify a registered-device ID for presence; authenticated device authorization is part of the auth milestone.

## Retry and abuse controls

Implemented now:

- POST mutations support idempotency keys;
- Redis-backed idempotency coordinates concurrent retries across API replicas;
- a reused key with a different request is rejected;
- 5xx results are not cached as successful mutations;
- configured Redis idempotency fails closed if coordination is unavailable;
- Redis-backed fixed-window limits share request budgets across replicas;
- rate-limit responses include `429`, `Retry-After`, `RateLimit-Limit` and `RateLimit-Remaining`;
- rate limiting fails open if Redis is unavailable because it is an abuse-control layer rather than a write-consistency boundary.

Still planned:

- authenticated per-account / per-device budgets;
- upload quotas;
- structured security events.

## Secret handling

This repository intentionally contains no App Store signing keys, Google service account credentials, APNs keys, FCM credentials, database production passwords or cloud object-storage secrets.
