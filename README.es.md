# PIXEL GO — Compartir entre dispositivos, nativo de verdad

[English](README.md) · [Español](README.es.md)

> **MÁNDALO. RECÍBELO DONDE QUIERAS.**  
> PIXEL GO mueve **archivos, fotos, links, texto y clipboard** entre iPhone y Android con dos clientes nativos independientes, un contrato versionado y un backend diseñado alrededor de garantías de entrega.

[![CI](https://github.com/armaabetancourtt/pixelgo/actions/workflows/ci.yml/badge.svg)](https://github.com/armaabetancourtt/pixelgo/actions/workflows/ci.yml)
![iOS](https://img.shields.io/badge/iOS-Swift_6_%7C_SwiftUI-000000?logo=apple&logoColor=white)
![Android](https://img.shields.io/badge/Android-Kotlin_%7C_Compose-3DDC84?logo=android&logoColor=white)
![Backend](https://img.shields.io/badge/Backend-Go-00ADD8?logo=go&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-Estado_Durable-4169E1?logo=postgresql&logoColor=white)
![Redis](https://img.shields.io/badge/Redis-Realtime_%7C_Idempotency-DC382D?logo=redis&logoColor=white)

## El producto

AirDrop es excelente cuando todos tus dispositivos viven dentro del mismo ecosistema. PIXEL GO explora qué pasa cuando no.

~~~text
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
                                             verifica SHA-256
                                                   ↓
                                              Delivered ✓
~~~

La superficie visible es pequeña a propósito. El reto vive debajo:

- dos clientes mobile realmente nativos;
- aislamiento por usuario y ownership de recursos;
- access tokens cortos y refresh tokens rotativos;
- retries móviles ambiguos e idempotencia;
- estado durable vs. presencia efímera;
- realtime entre réplicas;
- URLs firmadas e integridad binaria;
- compatibilidad de APIs mobile;
- CI/CD para Swift, Kotlin y Go.

## Un producto. Dos apps nativas.

PIXEL GO comparte **cero implementación de UI**.

~~~text
                         PIXEL GO API
                              │
                       OpenAPI Contract
                         ↙          ↘
                 Swift client    Kotlin client
                      ↓               ↓
                  SwiftUI App     Compose App
~~~

| Problema | iOS | Android |
|---|---|---|
| Lenguaje | Swift 6 | Kotlin |
| UI | SwiftUI | Jetpack Compose |
| Async | Swift Concurrency | Coroutines |
| Networking | URLSession + actor | HTTP nativo + Coroutines |
| Sesión segura | Keychain | Android Keystore + blob cifrado |
| Refresh concurrente | Task compartido | Mutex |
| Push | APNs | FCM |
| Background | BackgroundTasks | WorkManager |
| Persistencia local | frontera nativa | Room |
| Lifecycle | app / scene | activity / process |

Las dos apps ya tienen login/registro nativo, restauran la sesión segura, envían Bearer tokens, rotan refresh tokens y consumen devices, transfers y presencia Online / Offline del mismo API Go.

**Mismo producto. Dos implementaciones nativas reales.**

## Auth de verdad

~~~text
email + password
      ↓
bcrypt
      ↓
JWT access token · 15 min
      +
refresh token opaco · 30 días
      ↓
sólo SHA-256 del refresh se guarda en PostgreSQL
      ↓
cada refresh rota el token
      ↓
reusar un token viejo
      ↓
revoca toda la familia
~~~

Propiedades:

- passwords con bcrypt;
- JWT HS256 con issuer, audience, expiry, subject y JTI;
- refresh tokens opacos y aleatorios;
- el refresh nunca se persiste en claro;
- rotación transaccional en PostgreSQL;
- reuse detection revoca la familia;
- iOS guarda sesión en Keychain;
- Android cifra sesión con AES-GCM usando una key de Keystore;
- devices y transfers se filtran por usuario;
- una cuenta no puede transferir usando devices de otra;
- WebSocket y presence validan ownership.

Ver [docs/AUTH.md](docs/AUTH.md).

## Lifecycle de transferencia

~~~text
created → uploading → ready → downloading → completed
    └──────────────→ failed ←────────────────┘
~~~

El E2E ejecuta:

~~~text
Crear cuenta
     ↓
Registrar iPhone
     ↓
Registrar Android
     ↓
crear transfer
     ↓
signed upload URL
     ↓
PUT bytes reales
     ↓
validar tamaño + SHA-256
     ↓
transfer.ready
     ↓
signed download URL
     ↓
GET mismos bytes
     ↓
SHA-256
     ↓
complete
     ↓
Delivered ✓
~~~

El adapter local mantiene payload bytes en memoria para desarrollo/CI. La frontera está separada para reemplazarlo por S3/MinIO sin cambiar el dominio.

## Idempotencia distribuida

Un timeout puede ocurrir aunque el servidor ya haya confirmado la mutación.

PIXEL GO soporta Idempotency-Key:

- mismo usuario + misma key + mismo request → replay;
- misma key con request diferente → 409;
- retries concurrentes se coalescen;
- 5xx no queda guardado como éxito;
- replay expone Idempotency-Replayed;
- las keys están namespaced por usuario.

Con Redis, esto funciona entre **réplicas distintas del API**. Si Redis está configurado pero no disponible, idempotency falla cerrada con 503 antes que arriesgar una escritura duplicada.

## Arquitectura

~~~mermaid
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
    C[contracts/openapi.yaml] -. compatibilidad .-> I
    C -. compatibilidad .-> A
    C -. compatibilidad .-> API
~~~

### Estado durable vs. efímero

**PostgreSQL**

- users;
- hashes bcrypt;
- familias de refresh tokens hasheados;
- devices;
- lifecycle de transfers.

**Redis**

- presence TTL;
- Pub/Sub realtime;
- locks/resultados de idempotencia;
- rate limits compartidos.

Las signed URLs se regeneran y no se persisten como credenciales durables.

## Presence realtime

~~~text
WebSocket + deviceId propio
          ↓
Redis presence TTL = 45s
          ↓
heartbeat
          ↓
disconnect / expiry
          ↓
offline
~~~

Redis Pub/Sub permite que un evento generado en una réplica llegue a un cliente conectado a otra.

## Rate limiting

Redis comparte budgets y expone 429, Retry-After, RateLimit-Limit y RateLimit-Remaining.

Rate limiting falla abierto si Redis cae porque es control de abuso. Idempotency falla cerrado porque protege consistencia.

## API

~~~text
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

GET    /v1/events?deviceId=...
~~~

OpenAPI define Bearer auth y funciona como contrato de compatibilidad. CI en PRs busca cambios incompatibles.

## iOS nativo

Implementado:

- Swift 6 + SwiftUI;
- API client actor-isolated;
- login / register;
- Bearer auth;
- refresh automática;
- coalescing de refresh concurrente;
- sesión en Keychain;
- devices/transfers/presence reales;
- BackgroundTasks;
- XCTest + contract tests;
- XcodeGen.

## Android nativo

Implementado:

- Kotlin + Compose;
- Coroutines;
- login / register;
- Bearer auth;
- refresh automática;
- Mutex para refresh concurrente;
- AES-GCM + Android Keystore;
- devices/transfers/presence reales;
- WorkManager;
- Room boundary;
- FCM boundary;
- JUnit.

## CI/CD

~~~text
                         PR / PUSH
                             ↓
                   OpenAPI validation
                             ↓
        ┌────────────────────┼────────────────────┐
        ↓                    ↓                    ↓
    iOS build           Android build       Go vet + race
    Swift tests         JUnit               Postgres + Redis
        │                    │                    ↓
        │                    │          authenticated E2E
        └────────────────────┴────────────── Docker build
~~~

El E2E corre con auth obligatorio y prueba:

1. creación de cuenta;
2. rechazo sin Bearer;
3. iPhone + Android scoped a la cuenta;
4. transfer idempotente;
5. bytes reales upload/download;
6. SHA-256;
7. completed;
8. otra cuenta no ve los recursos;
9. refresh rotation;
10. reuse del refresh viejo revoca la familia;
11. el estado autenticado sobrevive restart del API.

## Estado actual

### Implementado

- SwiftUI y Compose nativos;
- auth UI en ambas plataformas;
- sesiones Keychain / Keystore;
- JWT + refresh rotation;
- reuse family revocation;
- user/resource isolation;
- OpenAPI;
- backend Go modular;
- PostgreSQL;
- Redis presence/PubSub/idempotency/rate limiting;
- WebSockets;
- signed binary local adapter;
- size + SHA-256;
- retries cross-replica;
- E2E autenticado;
- CI multiplataforma;
- Docker;
- release artifacts;
- docs EN/ES.

### Pendiente explícitamente

- adapter productivo S3/MinIO;
- APNs / FCM reales;
- UX completa de SEND + picker/upload;
- auto-download en background;
- E2EE de contenido;
- E2E físico iPhone → Android;
- TestFlight / Play con credenciales reales.

PIXEL GO no intenta demostrar 40 features.

Intenta demostrar que una idea sencilla puede ejecutarse con **profundidad de ingeniería ridículamente alta**.

**Un producto. Dos apps nativas. Un contrato.**
