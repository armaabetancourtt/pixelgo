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
    API --> FILES[Signed File Boundary]
    FILES --> OBJ[(S3 / MinIO adapter)]
    RT <--> REDIS

    CONTRACT[OpenAPI] -. compatibility .-> IOS
    CONTRACT -. compatibility .-> AND
    CONTRACT -. compatibility .-> API
~~~

## Native clients

### iOS

SwiftUI owns presentation. Swift Concurrency coordinates networking. The API client is actor-isolated.

Session credentials are encoded into Keychain. Concurrent 401 responses share one refresh Task so refresh rotation is not accidentally performed twice.

BackgroundTasks and notification permission boundaries are native iOS concerns.

### Android

Jetpack Compose owns presentation. Coroutines coordinate networking.

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
- transfer metadata and lifecycle state.

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

## File boundary

The current local adapter creates expiring HMAC-signed upload/download capability URLs.

The upload path verifies exact byte count and SHA-256 before the transfer can become ready.

The interface is intentionally isolated from the transfer domain so a direct S3-compatible adapter can replace it later.

Signed URLs are transient credentials and are not stored in PostgreSQL.

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

## Transfer path

~~~mermaid
sequenceDiagram
    participant S as Sender
    participant API as API
    participant F as File Boundary
    participant R as Redis / Realtime
    participant D as Destination

    S->>API: POST /v1/transfers + Idempotency-Key
    API-->>S: transfer + signed upload URL
    S->>F: PUT payload bytes
    S->>API: POST /uploaded
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
  ├── devices
  └── transfer metadata

Redis
  ├── presence TTL
  ├── realtime Pub/Sub
  ├── idempotency coordination
  └── rate-limit counters

Secure mobile storage
  ├── iOS Keychain
  └── Android Keystore-encrypted session

File adapter
  └── payload bytes
~~~

## Scaling path

The modular monolith stays one deployable backend while domain boundaries remain explicit.

Natural extraction points, if measurement justifies them:

1. realtime connection gateway;
2. background workers;
3. object-storage processing.

Auth, devices and transfers do not need artificial microservices to demonstrate sound boundaries.
