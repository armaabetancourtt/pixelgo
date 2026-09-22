# Authentication and session rotation

PIXEL GO treats authentication as a mobile-session problem, not just a login endpoint.

## Session shape

A successful register or login returns two credentials:

- a short-lived access JWT;
- a long-lived opaque refresh token.

~~~text
credentials
    ↓
bcrypt verification
    ↓
access JWT · 15 minutes
refresh token · 30 days
~~~

The access JWT contains standard registered claims including issuer, audience, subject, expiration and JTI.

Refresh tokens are random opaque values. They are not JWTs and the server never stores them in plaintext.

## Refresh storage

Before persistence, a refresh token is reduced to a SHA-256 hash.

PostgreSQL stores:

- token hash;
- user ID;
- token family ID;
- expiration;
- used timestamp;
- revoked timestamp.

Knowing the database value is therefore not enough to replay the original refresh credential.

## Rotation

Every successful refresh consumes the old token and creates a replacement in the same family.

~~~text
refresh A
   ↓ use
mark A used
   ↓
create refresh B
   ↓
return B
~~~

The PostgreSQL adapter performs this inside a transaction while locking the presented refresh row.

## Reuse detection

If refresh A is presented again after it was already used, PIXEL GO treats that as possible credential theft.

~~~text
A used successfully
      ↓
B issued
      ↓
A appears again
      ↓
revoke family
      ↓
B can no longer refresh
~~~

The API returns a specific refresh-reuse error for the replayed token. Subsequent tokens from the revoked family are invalid.

The E2E test exercises this exact flow.

## Mobile storage

### iOS

The complete TokenPair is encoded and stored in Keychain with a device-only accessibility policy.

The actor-based API client:

1. adds Bearer access credentials;
2. detects HTTP 401;
3. coalesces concurrent refresh attempts behind one shared Task;
4. rotates the refresh token;
5. persists the replacement TokenPair;
6. retries the original request once.

If refresh fails, the Keychain session is cleared.

### Android

The TokenPair is serialized locally, encrypted with AES-GCM and persisted as ciphertext.

The encryption key is created and retained by Android Keystore.

The coroutine API client:

1. sends Bearer access credentials;
2. detects HTTP 401;
3. enters a refresh Mutex;
4. checks whether another request already refreshed;
5. rotates only when necessary;
6. saves the new encrypted TokenPair;
7. retries the request.

This prevents two simultaneous 401 responses from accidentally rotating the same refresh token twice.

## Authorization

Authentication alone is not enough.

The authenticated user ID is added to request context and repository operations use it to scope data.

Current ownership rules:

- a user sees only their devices;
- a user sees only their transfers;
- transfer source and destination devices must both belong to that user;
- presence lookups require ownership of the device;
- WebSocket connections require an owned device ID;
- idempotency keys are namespaced by authenticated user.

Cross-account resource access is tested in the backend E2E.

## Local development

Authentication can be made mandatory with:

~~~bash
PIXELGO_REQUIRE_AUTH=true
~~~

CI always enables this mode.

A development JWT secret can be supplied with PIXELGO_JWT_SECRET. Production deployments must inject a strong secret through the deployment environment and must not use the example value from source control.

## Deliberate boundaries

Implemented:

- register;
- login;
- bcrypt;
- JWT access tokens;
- opaque refresh tokens;
- transactional rotation;
- reuse detection;
- family revocation;
- per-user resource ownership;
- native secure session storage.

Not yet claimed:

- password reset;
- email verification;
- MFA/passkeys;
- external identity providers;
- server-side access-token denylisting.

Those can be added independently without changing the transfer model.
