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

Current local/E2E transfer protection:

- short-lived HMAC-signed upload/download URLs;
- signatures scoped to action + transfer ID + expiry;
- transfer-state checks before upload/download;
- exact payload-size verification;
- SHA-256 integrity validation;
- user-scoped transfer metadata.

The current payload adapter is deliberately local/in-memory. Production object storage remains a separate adapter milestone.

This repository does not claim end-to-end content encryption. TLS protects transport; SHA-256 validates content integrity.

## Network

- TLS is required outside local development.
- cleartext endpoints exist only for simulator/emulator development;
- protected REST routes require Bearer access tokens when auth is enabled;
- WebSocket upgrades require an authenticated user and owned device ID;
- signed file routes use capability URLs rather than Bearer auth.

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

## Remaining security milestones

- production S3/MinIO object-storage adapter;
- real APNs/FCM delivery credentials;
- upload quotas;
- structured security audit events;
- password reset and account-recovery flows;
- optional MFA/passkeys;
- content end-to-end encryption if the product threat model requires it.
