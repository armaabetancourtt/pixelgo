# PIXEL GO — Native Cross-Device Sharing

[English](README.md) · [Español](README.es.md)

> **SEND IT. PICK IT UP ANYWHERE.**  
> PIXEL GO is a cross-device sharing system for moving **files, photos, links, text and clipboard content** between iPhone and Android — with two independent native clients, one versioned contract and a backend built around delivery guarantees rather than demo-only CRUD.

[![CI](https://github.com/armaabetancourtt/pixelgo/actions/workflows/ci.yml/badge.svg)](https://github.com/armaabetancourtt/pixelgo/actions/workflows/ci.yml)
![iOS](https://img.shields.io/badge/iOS-Swift_6_%7C_SwiftUI-000000?logo=apple&logoColor=white)
![Android](https://img.shields.io/badge/Android-Kotlin_%7C_Compose-3DDC84?logo=android&logoColor=white)
![Backend](https://img.shields.io/badge/Backend-Go-00ADD8?logo=go&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-Durable_State-4169E1?logo=postgresql&logoColor=white)
![Redis](https://img.shields.io/badge/Redis-Realtime_%7C_Idempotency-DC382D?logo=redis&logoColor=white)
![OpenAPI](https://img.shields.io/badge/OpenAPI-Compatibility_Contract-6BA539?logo=openapiinitiative&logoColor=white)

## The product

AirDrop is excellent when every device lives inside one ecosystem. PIXEL GO explores the engineering problem that appears when they do not.

```text
iPhone                                      Pixel

                PIXEL GO
                Copy / Send
                   photo
                     ↓
             signed upload URL
                     ↓
                 SHA-256
                     ↓
              transfer.ready
                     ├──────── WebSocket ────────→
                     └──────── push wake-up ────→
                                                   ↓
                                                download
                                                   ↓
                                             verify SHA-256
                                                   ↓
                                              Delivered ✓
```

The visible product stays deliberately small. That leaves room to go deep on the parts that are easy to list on a résumé and much harder to make coherent in a real system:

- two genuinely native mobile implementations;
- account and resource isolation;
- short-lived access tokens and rotating refresh tokens;
- ambiguous mobile retries and idempotency;
- durable state versus ephemeral presence;
- realtime delivery across multiple API replicas;
- signed binary transfer URLs and integrity validation;
- backwards-compatible API evolution;
- CI/CD across Swift, Kotlin and Go.

## One product. Two native apps.

PIXEL GO intentionally shares **no UI implementation** between platforms.

```text
                         PIXEL GO API
                              │
                       OpenAPI Contract
                         ↙          ↘
                 Swift client    Kotlin client
                      ↓               ↓
                  SwiftUI App     Compose App
```

| Concern | iOS | Android |
|---|---|---|
| Language | Swift 6 | Kotlin |
| UI | SwiftUI | Jetpack Compose |
| Async | Swift Concurrency | Coroutines |
| Networking | URLSession actor | native HTTP + Coroutines |
| Session storage | Keychain | Android Keystore + encrypted local blob |
| Refresh coordination | actor-shared refresh task | coroutine Mutex |
| Push boundary | APNs | FCM |
| Background work | BackgroundTasks | WorkManager |
| Local data boundary | native persistence boundary | Room |
| Lifecycle | app / scene | activity / process |

Both apps now implement native sign-in / account creation, restore encrypted sessions, send Bearer access tokens, rotate refresh tokens when needed and load devices, transfers and Online / Offline presence from the same Go API.

**Same product semantics. Separate native codebases.**

## Authentication is part of the system, not a badge

PIXEL GO implements a real session model:

```text
email + password
      ↓
bcrypt password hash
      ↓
15 minute access JWT
      +
30 day opaque refresh token
      ↓
refresh token stored only as SHA-256 hash in PostgreSQL
      ↓
refresh rotates on every use
      ↓
reuse of an old refresh token
      ↓
entire token family revoked
```

Important properties:

- passwords are hashed with bcrypt;
- access tokens are HS256 JWTs with issuer, audience, expiry, subject and JTI;
- refresh tokens are opaque random values, never JWTs;
- only refresh-token hashes are stored server-side;
- rotation is transactional in PostgreSQL;
- reuse of an already-consumed refresh token is treated as a possible stolen-token signal and revokes the family;
- iOS stores the session in Keychain;
- Android encrypts the session with an AES-GCM key held by Android Keystore;
- resource repositories scope devices and transfers to the authenticated user;
- a user cannot create a transfer using another account's devices;
- WebSocket device identity and presence lookups are ownership-checked.

See [docs/AUTH.md](docs/AUTH.md).

## Transfer lifecycle

Transfers are modeled as explicit state instead of one giant upload request.

```text
created → uploading → ready → downloading → completed
    └──────────────→ failed ←────────────────┘
```

The executable local/CI flow is:

```text
Create account
     ↓
Register iPhone
     ↓
Register Android
     ↓
POST /v1/transfers
     ↓
receive expiring HMAC-signed upload URL
     ↓
PUT real payload bytes
     ↓
server verifies declared size + SHA-256
     ↓
POST /uploaded
     ↓
transfer.ready
     ↓
receive signed download URL
     ↓
GET the same bytes
     ↓
validate SHA-256
     ↓
POST /complete
     ↓
transfer.completed
     ↓
Delivered ✓
```

For deterministic local development and CI, the current payload adapter stores bytes in process memory behind signed URLs. The file boundary is separated so a production S3/MinIO adapter can replace it without changing transfer-domain semantics.

## Retry safety across replicas

Mobile failures are ambiguous. A request can time out after the server already committed it.

PIXEL GO supports `Idempotency-Key` on POST mutations.

Current behavior:

- same user + same key + same request → replay original response;
- same key reused with a different request → `409 idempotency_key_reused`;
- concurrent identical retries are coalesced;
- 5xx responses are not committed as successful idempotency records;
- replayed responses include `Idempotency-Replayed: true`;
- idempotency keys are namespaced by authenticated user.

When Redis is configured, coordination is **cross-replica**. Two retries can hit two API instances and still execute the underlying mutation once. Redis uses an owner-scoped lock plus a replayable result record. If distributed idempotency is configured but Redis is unavailable, mutation coordination fails closed with `503` instead of risking a duplicate write.

## Architecture

```mermaid
flowchart LR
    I[iOS · SwiftUI] -->|JWT + REST| API[Go API]
    A[Android · Compose] -->|JWT + REST| API

    I <--> |WebSocket| RT[Realtime Hub]
    A <--> |WebSocket| RT

    API --> PG[(PostgreSQL)]
    API --> R[(Redis)]
    API --> FS[Signed File Boundary]
    FS --> O[(S3 / MinIO production adapter)]
    API --> W[Workers]
    W --> P[APNs / FCM]
    RT <--> R

    C[contracts/openapi.yaml] -. compatibility .-> I
    C -. compatibility .-> A
    C -. compatibility .-> API
```

### Durable versus ephemeral state

The split is executable, not just diagrammed.

**PostgreSQL**

- users;
- bcrypt password hashes;
- hashed refresh-token families;
- registered devices;
- transfer metadata and lifecycle state.

**Redis**

- device presence TTLs;
- realtime Pub/Sub fan-out across API replicas;
- cross-replica idempotency locks and replay records;
- shared fixed-window rate limits.

**File boundary**

- expiring signed upload/download capabilities;
- local in-memory bytes today;
- S3-compatible object storage is the next adapter.

Signed URLs are regenerated from transfer state and are deliberately **not persisted as durable credentials**.

## Realtime presence

A registered device is durable. Being online is not.

```text
WebSocket connects with owned deviceId
               ↓
        Redis presence key
            TTL = 45s
               ↓
         heartbeat refresh
               ↓
disconnect / TTL expiry
               ↓
            offline
```

Redis Pub/Sub lets a transfer event produced by one API replica reach WebSocket clients connected to another replica.

## Rate limiting

Redis-backed fixed windows share request budgets across replicas. HTTP responses expose:

- `429 Too Many Requests`;
- `Retry-After`;
- `RateLimit-Limit`;
- `RateLimit-Remaining`.

Unlike idempotency, rate limiting is an abuse-control layer rather than a consistency boundary, so it intentionally fails open if Redis becomes unavailable.

## Repository layout

```text
pixelgo/
├── ios/                         # Swift 6 / SwiftUI
├── android/                     # Kotlin / Jetpack Compose
├── server/
│   ├── cmd/api/
│   └── internal/
│       ├── auth/
│       ├── devices/
│       ├── transfers/
│       ├── files/
│       ├── presence/
│       ├── realtime/
│       ├── ratelimit/
│       ├── httpapi/
│       └── platform/
├── contracts/
│   └── openapi.yaml
├── infrastructure/
│   ├── docker-compose.yml
│   └── postgres/
├── docs/
│   ├── ARCHITECTURE.md
│   ├── AUTH.md
│   ├── TRANSFER_LIFECYCLE.md
│   ├── SECURITY.md
│   └── LOCAL_DEVELOPMENT.md
├── tests/e2e/
└── .github/workflows/
```

## API contract

`contracts/openapi.yaml` is the compatibility boundary between software that cannot be deployed atomically.

Core routes:

```text
GET    /health

POST   /v1/auth/register
POST   /v1/auth/login
POST   /v1/auth/refresh

GET    /v1/devices
POST   /v1/devices
DELETE /v1/devices/{deviceId}
GET    /v1/presence/{deviceId}

GET    /v1/transfers
POST   /v1/transfers
GET    /v1/transfers/{transferId}
POST   /v1/transfers/{transferId}/uploaded
POST   /v1/transfers/{transferId}/complete

GET    /v1/events?deviceId=...    # authenticated WebSocket upgrade
```

Protected routes use the OpenAPI Bearer security scheme. Pull-request CI validates the contract and rejects incompatible API changes against the base revision.

## Native iOS

Implemented:

- Swift 6 + SwiftUI;
- actor-isolated API client;
- native login / register UI;
- Bearer authentication;
- automatic refresh-token rotation;
- concurrent refresh coalescing;
- Keychain session persistence;
- live API devices / transfers / presence;
- BackgroundTasks boundary;
- notification permission boundary;
- XCTest + host contract tests;
- reproducible XcodeGen project.

## Native Android

Implemented:

- Kotlin + Jetpack Compose;
- Coroutines;
- native login / register UI;
- Bearer authentication;
- automatic refresh-token rotation;
- coroutine `Mutex` refresh coalescing;
- AES-GCM session encryption using Android Keystore;
- live API devices / transfers / presence;
- WorkManager boundary;
- Room persistence boundary;
- FCM dependency boundary;
- JUnit;
- AndroidX build configuration.

## Security boundaries

Current transfer security:

- HMAC-SHA256 signed local transfer URLs;
- action-specific upload/download signatures;
- short expiry;
- exact byte-count verification;
- SHA-256 payload validation;
- user-scoped transfer metadata;
- access/refresh token separation;
- refresh-token family revocation.

This is **not yet end-to-end file encryption**. TLS protects transport and SHA-256 proves integrity in the current design. Content E2EE would be a separate cryptographic protocol and is not claimed here.

See [docs/SECURITY.md](docs/SECURITY.md).

## CI/CD

Every push / PR validates independently built software:

```text
                         PR / PUSH
                             ↓
                   OpenAPI validation
                             ↓
        ┌────────────────────┼────────────────────┐
        ↓                    ↓                    ↓
   iOS native build     Android build       Go vet + race
   Swift tests          JUnit tests         PostgreSQL + Redis
        │                    │                    ↓
        │                    │          authenticated binary E2E
        │                    │                    ↓
        └────────────────────┴────────────── Docker build
```

The backend E2E runs with **authentication required**, PostgreSQL and Redis enabled. It proves:

1. account creation;
2. unauthenticated requests are rejected;
3. user-scoped iPhone + Android registration;
4. retry-safe transfer creation;
5. real signed upload/download bytes;
6. SHA-256 integrity;
7. completed delivery state;
8. another account cannot see the devices or transfer;
9. refresh rotation;
10. old refresh-token reuse revokes the family;
11. durable authenticated state survives an API restart.

Release tags build independent backend, Android and iOS artifacts. Real store/deployment credentials are intentionally not committed.

## Testing philosophy

The goal is not a vanity test count. Tests target invariants that would hurt real users if broken:

- auth isolation;
- bcrypt credential verification;
- refresh reuse detection;
- transfer state transitions;
- signed URL tamper rejection;
- checksum and size mismatch;
- sequential and concurrent idempotency;
- cross-replica retry coordination;
- PostgreSQL restart persistence;
- Redis presence expiry;
- Redis Pub/Sub fan-out;
- shared rate-limit counters;
- mobile contract decoding.

## Local development

Start infrastructure:

```bash
docker compose -f infrastructure/docker-compose.yml up -d
```

Backend:

```bash
cd server
cp .env.example .env
go mod tidy
go test ./...
go run ./cmd/api
```

To run the same security path as CI, set:

```bash
PIXELGO_REQUIRE_AUTH=true
```

Then run:

```bash
python3 tests/e2e/transfer_flow.py
```

iOS:

```bash
cd ios
brew install xcodegen
xcodegen generate
open PixelGo.xcodeproj
```

Android:

```bash
cd android
gradle :app:testDebugUnitTest
gradle :app:assembleDebug
```

Full setup: [docs/LOCAL_DEVELOPMENT.md](docs/LOCAL_DEVELOPMENT.md).

## Current status

### Implemented

- native SwiftUI and Compose applications;
- native auth UI on both platforms;
- Keychain / Keystore-backed sessions;
- JWT access tokens + rotating opaque refresh tokens;
- refresh-token reuse family revocation;
- authenticated user/resource isolation;
- versioned OpenAPI contract;
- Go modular monolith;
- PostgreSQL runtime repositories;
- Redis presence, Pub/Sub, idempotency and rate limiting;
- WebSocket realtime hub;
- signed binary upload/download development adapter;
- exact-size and SHA-256 validation;
- cross-replica retry safety;
- authenticated E2E lifecycle;
- cross-platform CI;
- Docker image build;
- release artifacts;
- bilingual engineering documentation.

### Intentionally still pending

- production S3/MinIO direct object-storage adapter;
- real APNs / FCM delivery adapters and push credentials;
- complete mobile SEND picker/upload UX;
- background destination auto-download;
- content end-to-end encryption;
- physical iPhone → Android automated E2E;
- real TestFlight / Play internal-distribution credentials.

PIXEL GO is intentionally narrower than a social network, map product or ML platform.

Its purpose remains simple:

> **Send something from one device to another.**

That small product surface creates room to demonstrate the engineering underneath with unusual depth.

**One product. Two native apps. One contract.**
