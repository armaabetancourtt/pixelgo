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
- WebSocket authentication uses the same account/device authorization model as REST.

## Abuse controls

Production milestones include:

- per-account and per-device rate limits;
- IP-aware abuse limits;
- upload quotas;
- maximum payload sizes;
- idempotency-key replay handling;
- structured security events.

## Secret handling

This repository intentionally contains no App Store signing keys, Google service account credentials, APNs keys, FCM credentials, database production passwords or cloud object-storage secrets.
