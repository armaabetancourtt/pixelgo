<p align="center"><img src="../brand/pixelgo-banner.svg" width="680" alt="PixelGo official pastel identity" /></p>

# Architecture

PIXEL GO is a modular monolith with two independently implemented native clients.

## Principles

1. Native clients own native UX and lifecycle behavior.
2. OpenAPI is the compatibility boundary.
3. Authentication and authorization are separate concerns.
4. Durable identity/state and ephemeral coordination use different stores.
5. Binary payloads should bypass the application server in production.
6. Event delivery may be at-least-once; mutations must be idempotent.
7. Mobile clients must tolerate server evolution, offline periods and ambiguous retries.

## High-level topology

~~~mermaid
flowchart LR
    IOS[iOS · SwiftUI] -->|JWT + REST| API[Go API]
    AND[Android · Compose] -->|JWT + REST| API
    IOS <--> |WebSocket| RT[Realtime Hub]
    AND <--> |WebSocket| RT

    API --> PG[(PostgreSQL)]
    API --> REDIS[(Redis)]
    API --> FILES[Presigned Object Boundary]
    IOS -->|direct PUT / GET| OBJ[(S3 / MinIO)]
    AND -->|direct PUT / GET| OBJ
    FILES --> OBJ
    RT <--> REDIS

    CONTRACT[OpenAPI] -. compatibility .-> IOS
    CONTRACT -. compatibility .-> AND
    CONTRACT -. compatibility .-> API
~~~

## Native clients

### iOS

SwiftUI owns presentation. Swift Concurrency coordinates networking. Native PhotosPicker/fileImporter flows send photos and files, while an explicit clipboard action sends clipboard text as its own transfer kind. The API client is actor-isolated.

Session credentials are encoded into Keychain. Concurrent 401 responses share one refresh Task so refresh rotation is not accidentally performed twice.

BackgroundTasks and notification permission boundaries are native iOS concerns.

### Android

Jetpack Compose owns presentation. Coroutines coordinate networking. Native Photo Picker/document flows send binary payloads and a clipboard action uses the same transfer pipeline for clipboard text.

The session is encrypted with AES-GCM and the key lives in Android Keystore. A coroutine Mutex coalesces refresh rotation after concurrent 401 responses.

WorkManager, Room and FCM remain Android-specific boundaries.

## Authentication and authorization

Register/login issue:

- a 15-minute JWT access token;
- a 30-day opaque refresh token.

Refresh tokens are stored only as hashes and rotate on every use.

The authenticated user ID is placed in request context. PostgreSQL and memory repositories scope device/transfer operations to that user whenever the context is authenticated.

Transfer creation verifies both source and destination devices belong to the caller.

See [AUTH.md](AUTH.md).

## PostgreSQL

PostgreSQL is the durable source of truth when DATABASE_URL is configured.

It stores:

- users;
- bcrypt password hashes;
- hashed refresh-token families;
- devices;
- transfer metadata and lifecycle state;
- durable notification outbox rows.

CI starts a real PostgreSQL service and proves authenticated state survives an API process restart.

The backend can use memory repositories when DATABASE_URL is absent for isolated local tests.

## Redis

Redis owns ephemeral/distributed coordination when REDIS_URL is configured:

- device presence with TTL heartbeats;
- Pub/Sub event fan-out across API replicas;
- idempotency locks + replay records;
- shared fixed-window request limits.

The design intentionally uses different failure policies:

- idempotency fails closed because duplicate writes risk consistency;
- rate limiting fails open because it is an abuse-control layer.

## Object-storage boundary

When `OBJECT_STORAGE_ENDPOINT` is configured, the file service uses an S3-compatible adapter. The API presigns PUT/GET capabilities and payload bytes move directly between native clients and object storage.

The presigned PUT binds `X-Amz-Meta-Sha256` into the SigV4 signature. After upload, `POST /uploaded` causes the transfer domain to `HEAD` the object and verify:

- the object exists;
- exact byte size matches transfer metadata;
- signed SHA-256 metadata matches the declared checksum.

Only then can the transfer transition to `ready`.

When object storage is absent, a local HMAC-signed in-memory adapter remains available for isolated development/tests. Signed URLs are transient credentials and are not stored in PostgreSQL.

## Realtime path

~~~text
authenticated client
      ↓
owned deviceId
      ↓
WebSocket
      ↓
Redis presence TTL
      ↓
Redis Pub/Sub
      ↓
other API replicas
      ↓
connected destination client
~~~

A device registration is durable. Presence is an observation with expiry.

## Push wake-up path

Realtime remains the preferred delivery signal. Push exists to wake a destination that is not currently connected.

~~~text
transfer status update → ready
          ↓
PostgreSQL trigger
          ↓
notification_outbox
          ↓
worker claims rows with FOR UPDATE SKIP LOCKED
          ↓
Redis presence check
     ↙           ↘
 online         offline / unknown
   ↓                 ↓
skip push        APNs / FCM
                      ↓
                sent / retry
~~~

The outbox insert and transfer state transition share the PostgreSQL transaction, so a process crash after commit cannot lose the wake-up intent.

The worker is safe to run on multiple replicas: claimed rows are locked durably, stale locks can be reclaimed, and provider failures use bounded exponential backoff. Online devices are intentionally marked delivered by the outbox worker without a provider call because their WebSocket connection already receives `transfer.ready`.

Device-token rotation is independent from device registration through `PUT /v1/devices/{deviceId}/push-token`.

## Transfer path

~~~mermaid
sequenceDiagram
    participant S as Sender
    participant API as API
    participant F as S3 / MinIO
    participant R as Redis / Realtime
    participant D as Destination

    S->>API: POST /v1/transfers + Idempotency-Key
    API-->>S: transfer + signed upload URL
    S->>F: presigned PUT + SHA-256 metadata
    S->>API: POST /uploaded
    API->>F: HEAD object
    F-->>API: size + checksum metadata
    API->>R: transfer.ready
    R-->>D: realtime event
    D->>F: GET payload
    D->>D: verify SHA-256
    D->>API: POST /complete
    API->>R: transfer.completed
    R-->>S: Delivered
~~~

## State ownership

~~~text
PostgreSQL
  ├── users
  ├── password hashes
  ├── refresh-token families
  ├── devices + push tokens
  ├── transfer metadata
  └── notification outbox

Redis
  ├── presence TTL
  ├── realtime Pub/Sub
  ├── idempotency coordination
  └── rate-limit counters

Secure mobile storage
  ├── iOS Keychain
  └── Android Keystore-encrypted session

Object storage
  ├── payload bytes
  └── signed checksum metadata
~~~

## Observability

Observability wraps the HTTP stack without changing domain services.

~~~text
HTTP request
    ↓
request ID
    ↓
auth / idempotency / rate limiting / handler
    ↓
status + bytes + latency
    ├── structured JSON log
    └── Prometheus metrics
~~~

The metrics registry exposes `GET /metrics` in OpenMetrics-compatible format. Route labels are normalized before recording, so resource IDs such as transfer/device IDs never become metric-label cardinality.

The request logger records request ID, method, normalized route, status, duration and response byte count. It deliberately does not log Authorization headers or request bodies.

The response-writer wrapper preserves flushing, hijacking and unwrapping behavior so the WebSocket endpoint continues to work through the same middleware.

## Scaling path

The modular monolith stays one deployable backend while domain boundaries remain explicit.

Natural extraction points, if measurement justifies them:

1. realtime connection gateway;
2. background workers;
3. object-storage processing.

Auth, devices and transfers do not need artificial microservices to demonstrate sound boundaries.
