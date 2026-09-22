# Local development

## Prerequisites

- Docker + Docker Compose
- Go 1.25+
- Xcode + XcodeGen for iOS
- JDK 17 + Gradle for Android

## Infrastructure

From the repository root:

```bash
docker compose -f infrastructure/docker-compose.yml up -d
```

Local services:

- PostgreSQL: `localhost:5432`
- Redis: `localhost:6379`
- MinIO API: `localhost:9000`
- MinIO console: `localhost:9001`

These credentials are development-only and intentionally obvious.

## Backend

```bash
cd server
cp .env.example .env
go mod tidy
go test ./...
go run ./cmd/api
```

Check:

```bash
curl http://localhost:8080/health
```

Register two local devices:

```bash
curl -X POST http://localhost:8080/v1/devices \
  -H 'Content-Type: application/json' \
  -d '{"name":"Armando iPhone","platform":"ios"}'

curl -X POST http://localhost:8080/v1/devices \
  -H 'Content-Type: application/json' \
  -d '{"name":"Pixel 10","platform":"android"}'
```

## iOS

```bash
cd ios
brew install xcodegen
xcodegen generate
open PixelGo.xcodeproj
```

The simulator points to `http://localhost:8080`.

## Android

```bash
cd android
gradle :app:testDebugUnitTest
gradle :app:assembleDebug
```

The Android emulator points to `http://10.0.2.2:8080`, which maps to the host machine.

## Current limitation

The running API intentionally uses in-memory repositories in the first foundation commit. PostgreSQL, Redis and MinIO are provisioned locally now so the next milestone can replace those adapters without changing the domain contract.
