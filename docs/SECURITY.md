# Security model

PIXEL GO handles personal files, account credentials and device reachability, so security boundaries are part of the product model.

## Authentication

Implemented:

- bcrypt password hashing;
- short-lived HS256 access JWTs;
- issuer, audience, subject, expiry and JTI claims;
- opaque random refresh tokens;
- SHA-256 refresh-token persistence;
- transactional refresh rotation in PostgreSQL;
- refresh-token reuse detection;
- family revocation after reuse;
- user-scoped devices and transfers;
- ownership checks for presence and WebSocket device identity.

iOS stores the session in Keychain.

Android encrypts the session with AES-GCM using key material protected by Android Keystore.

See [AUTH.md](AUTH.md).

## Files

The configured production-style path uses S3-compatible object storage:

- short-lived presigned PUT/GET URLs;
- mobile payload bytes bypass the Go API;
- the PUT requires `X-Amz-Meta-Sha256` and that header is part of the SigV4 signature;
- `POST /uploaded` performs an authenticated object `HEAD`;
- exact object size must match transfer metadata;
- stored SHA-256 metadata must match the transfer checksum;
- the destination independently hashes downloaded bytes before marking delivery complete;
- transfer metadata remains user-scoped.

This prevents the sender from advancing a transfer to `ready` merely by calling the API without first placing the expected object in storage. It also makes checksum metadata tampering invalidate the signed PUT.

An HMAC-signed in-memory adapter remains available only when object storage is not configured.

This repository does not claim end-to-end content encryption. TLS protects transport; SHA-256 validates content integrity.

## Network

- TLS is required outside local development.
- cleartext endpoints exist only for simulator/emulator development;
- protected REST routes require Bearer access tokens when auth is enabled;
- WebSocket upgrades require an authenticated user and owned device ID;
- S3-compatible upload/download routes use short-lived capability URLs rather than Bearer auth;
- local fallback file routes use HMAC capability URLs and are disabled while S3-compatible storage is active.

## Retry safety

POST mutations support idempotency keys.

With Redis:

- equivalent concurrent retries are coalesced across API replicas;
- completed responses are replayed;
- the same key with a different request is rejected;
- idempotency is namespaced by authenticated user;
- 5xx responses are not committed as successful replay records;
- Redis coordination failure is fail-closed for protected mutations.

The fail-closed choice is intentional: duplicate writes are a consistency risk.

## Abuse controls

Redis-backed fixed-window limits share request budgets across replicas.

Responses expose:

- HTTP 429;
- Retry-After;
- RateLimit-Limit;
- RateLimit-Remaining.

Rate limiting fails open when Redis is unavailable because availability is preferred for this abuse-control layer. This is deliberately different from the idempotency failure policy.

## Durable and ephemeral secrets

PostgreSQL persists durable identity and transfer metadata.

Redis stores ephemeral coordination such as presence, request budgets and idempotency records.

Signed transfer URLs are generated from current transfer state and are not persisted as durable credentials.

## Secret handling

The repository intentionally contains no:

- production JWT signing secret;
- App Store signing certificates;
- APNs keys;
- FCM service-account credentials;
- production database password;
- production object-storage credential;
- Google Play signing material.

Example local values are development-only.

## Push credential boundaries

APNs signing keys and FCM service-account JSON are deployment secrets and are never stored in the repository. The backend accepts them only through environment configuration, with base64 used as transport encoding rather than as encryption.

Device push tokens are scoped to an authenticated device and can rotate without creating a new device record. Push payloads carry transfer identifiers and presentation metadata, never file bytes, signed storage URLs, access tokens or refresh tokens.

A PostgreSQL outbox persists delivery intent. Provider errors are logged without logging provider credentials or device push-token values.

## Logging and operational telemetry

Structured HTTP logs intentionally exclude:

- Authorization headers;
- refresh/access tokens;
- request bodies;
- file payloads;
- signed object-storage URLs.

Logs contain only operational metadata such as request ID, method, normalized route, status, duration and response byte count. Metric labels use normalized route templates rather than resource IDs to avoid accidental identifier leakage and unbounded cardinality.

The `/metrics` endpoint is designed for private-network scraping. Production ingress should not expose it directly to the public internet.

## Remaining security milestones

- production bucket policies / cloud IAM hardening;
- object lifecycle, retention and deletion policy;
- real APNs/FCM delivery credentials;
- upload quotas;
- structured security audit events;
- password reset and account-recovery flows;
- optional MFA/passkeys;
- content end-to-end encryption if the product threat model requires it.
