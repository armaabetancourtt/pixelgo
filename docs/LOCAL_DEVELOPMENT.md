# Local development

## Prerequisites

- Docker + Docker Compose
- Go 1.25+
- Python 3
- Xcode + XcodeGen for iOS
- JDK 17 + Gradle for Android

## Infrastructure

From repository root:

~~~bash
docker compose -f infrastructure/docker-compose.yml up -d
~~~

Services:

- PostgreSQL: localhost:5432
- Redis: localhost:6379
- MinIO API: localhost:9000
- MinIO console: localhost:9001

PostgreSQL and Redis are used automatically when DATABASE_URL and REDIS_URL are present.

MinIO is provisioned for the production-style storage milestone; the current signed local payload adapter remains in-process for deterministic E2E tests.

## Backend

~~~bash
cd server
cp .env.example .env
go mod tidy
go test ./...
go run ./cmd/api
~~~

Health:

~~~bash
curl http://localhost:8080/health
~~~

### Authentication mode

Local development can run with auth optional for fast infrastructure work.

To exercise the production-style path used by CI:

~~~bash
export PIXELGO_REQUIRE_AUTH=true
export PIXELGO_JWT_SECRET='local-development-secret-at-least-32-bytes'
~~~

Do not reuse the example JWT secret in a real deployment.

### Persistence modes

With DATABASE_URL:

- users persist;
- refresh-token families persist;
- devices persist;
- transfers persist.

Without DATABASE_URL, the same domain services use memory repositories.

With REDIS_URL:

- presence uses TTL keys;
- realtime uses Pub/Sub;
- idempotency is cross-replica;
- rate limits are shared.

Without REDIS_URL, isolated development uses in-process adapters where available.

## Authenticated transfer E2E

Start the backend with auth enabled, then from repository root:

~~~bash
python3 tests/e2e/transfer_flow.py
~~~

The E2E proves:

1. account registration;
2. protected route rejects missing Bearer;
3. iPhone and Android roles register under the account;
4. transfer creation is retry-safe;
5. bytes upload through a signed URL;
6. server validates size and SHA-256;
7. bytes download unchanged;
8. transfer reaches completed;
9. a second account cannot see the first account's resources;
10. refresh rotates;
11. reuse of the old refresh token revokes the family.

CI additionally restarts the API and logs in again to prove PostgreSQL state survived.

## iOS

~~~bash
cd ios
brew install xcodegen
xcodegen generate
open PixelGo.xcodeproj
~~~

Simulator API base URL is localhost:8080.

The app now includes native register/login UI and stores the TokenPair in Keychain.

Swift 6 concurrency checks stay enabled. The API client is actor-isolated and coalesces concurrent refresh work.

Host contract tests:

~~~bash
swift test
~~~

## Android

~~~bash
cd android
gradle :app:testDebugUnitTest
gradle :app:assembleDebug
~~~

The Android emulator reaches the host API at 10.0.2.2:8080.

The app includes native register/login UI. Session JSON is encrypted with AES-GCM before persistence; the AES key is held by Android Keystore.

Concurrent refresh attempts are serialized by a coroutine Mutex.

## File-development constraint

The current signed-payload development adapter stores file bytes in API process memory and is limited to 64 MiB.

That is intentionally not presented as the final storage architecture.

Next storage milestone:

- direct S3/MinIO presigned PUT/GET;
- object expiry/cleanup;
- production-size limits;
- background validation where required.

## Push-development constraint

APNs/FCM boundaries exist in the native projects, but real provider credentials and production push adapters are not committed.

The realtime WebSocket path is implemented independently of push wake-up.
