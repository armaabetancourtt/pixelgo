# Local development

## Prerequisites

- Docker + Docker Compose
- Go 1.25+
- Python 3 for the E2E script
- Xcode + XcodeGen for iOS
- JDK 17 + Gradle for Android

## Optional infrastructure

From the repository root:

```bash
docker compose -f infrastructure/docker-compose.yml up -d
```

Provisioned local services:

- PostgreSQL: `localhost:5432`
- Redis: `localhost:6379`
- MinIO API: `localhost:9000`
- MinIO console: `localhost:9001`

These services represent the production-oriented architecture, but the first runtime foundation still uses in-memory metadata/idempotency adapters and an in-memory signed payload adapter. That limitation is explicit rather than hidden.

## Backend

```bash
cd server
cp .env.example .env
go mod tidy
go test ./...
go run ./cmd/api
```

Health check:

```bash
curl http://localhost:8080/health
```

The local server uses `PIXELGO_SIGNING_SECRET` to generate short-lived HMAC-signed upload/download URLs. The value in `.env.example` is development-only.

## Run the real local transfer E2E

Keep the API running, then from the repository root:

```bash
python3 tests/e2e/transfer_flow.py
```

The script:

1. registers an iPhone role;
2. registers an Android role;
3. creates a transfer;
4. retries creation with the same idempotency key and verifies the same transfer ID;
5. uploads actual bytes through the signed upload URL;
6. confirms the upload;
7. downloads the actual bytes through the signed download URL;
8. validates SHA-256;
9. marks the transfer completed.

A passing run prints:

```text
PASS: real signed upload/download + SHA-256 + idempotent delivery
```

The local signed-payload adapter is limited to 64 MiB because it stores bytes in process memory. That is a development constraint, not the intended production upload limit.

## iOS

```bash
cd ios
brew install xcodegen
xcodegen generate
open PixelGo.xcodeproj
```

The iOS simulator points to `http://localhost:8080`.

The project uses Swift 6 concurrency checks. The BackgroundTasks coordinator is isolated to `MainActor` instead of disabling concurrency safety.

## Android

```bash
cd android
gradle :app:testDebugUnitTest
gradle :app:assembleDebug
```

The Android emulator points to `http://10.0.2.2:8080`, which maps to the host machine.

AndroidX is enabled in `android/gradle.properties`.

## Current runtime boundaries

Implemented locally:

- devices and transfer metadata in memory;
- WebSocket hub in process;
- idempotency records in memory;
- HMAC-signed upload/download URLs;
- payload bytes in memory;
- exact-size and SHA-256 verification.

Provisioned but not yet wired into the runtime:

- PostgreSQL repositories;
- Redis distributed presence/idempotency/rate limiting;
- MinIO/S3 production object-storage adapter.

The intent is to swap adapters without changing the transfer-domain contract.
