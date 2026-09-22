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

PostgreSQL and Redis are used automatically when `DATABASE_URL` and `REDIS_URL` are present.

MinIO is the S3-compatible payload store when `OBJECT_STORAGE_ENDPOINT` is set. The Compose file uses the official `quay.io/minio/minio` image. With the example local settings and `OBJECT_STORAGE_AUTO_CREATE=true`, the API creates the bucket if it does not exist.

The Go process does not automatically source `.env`; export/source those values in your shell or inject them through your process manager.

## Backend

~~~bash
cd server
cp .env.example .env
go mod tidy
go test ./...
go run ./cmd/api
~~~

Health and metrics:

~~~bash
curl -i http://localhost:8080/health
curl http://localhost:8080/metrics
~~~

Every HTTP response includes `X-Request-ID`. Request logs are JSON structured. Set `PIXELGO_LOG_LEVEL=debug` for verbose local request logging.

### Authentication mode

Local development can run with auth optional for fast infrastructure work.

To exercise the production-style path used by CI:

~~~bash
export PIXELGO_REQUIRE_AUTH=true
export PIXELGO_JWT_SECRET='local-development-secret-at-least-32-bytes'
export PIXELGO_LOG_LEVEL='debug'

export OBJECT_STORAGE_ENDPOINT='http://localhost:9000'
export OBJECT_STORAGE_BUCKET='pixelgo'
export OBJECT_STORAGE_ACCESS_KEY='pixelgo'
export OBJECT_STORAGE_SECRET_KEY='pixelgo-local-secret'
export OBJECT_STORAGE_REGION='us-east-1'
export OBJECT_STORAGE_PREFIX='transfers'
export OBJECT_STORAGE_AUTO_CREATE='true'
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
5. bytes upload directly to MinIO through a presigned PUT;
6. the signed PUT carries SHA-256 metadata;
7. the API HEADs MinIO and validates exact size + checksum before `ready`;
8. bytes download directly through a presigned GET and hash unchanged;
9. transfer reaches completed;
10. a second account cannot see the first account's resources;
11. refresh rotates;
12. reuse of the old refresh token revokes the family.

CI additionally restarts the API and logs in again to prove PostgreSQL metadata survived while the object remains independently durable in MinIO.

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

## Object storage modes

### S3 / MinIO mode

When `OBJECT_STORAGE_ENDPOINT` is configured:

- the API issues short-lived presigned PUT/GET URLs;
- clients upload/download directly to object storage;
- uploads must include the signed `X-Amz-Meta-Sha256` header;
- `POST /uploaded` performs an object HEAD;
- exact size + checksum metadata are verified before the transfer becomes ready.

This is the mode exercised by backend CI.

### Local fallback mode

When object storage is unset, the HMAC-signed development adapter stores payload bytes in the API process and exposes `/dev-upload` / `/dev-download`. It is limited to 64 MiB and exists for isolated development/tests.

Remaining storage work is operational rather than architectural:

- lifecycle/expiry cleanup;
- production IAM and bucket policies;
- quotas;
- multipart/background behavior for very large payloads.

## Push-development constraint

APNs/FCM boundaries exist in the native projects, but real provider credentials and production push adapters are not committed.

The realtime WebSocket path is implemented independently of push wake-up.
