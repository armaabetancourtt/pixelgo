<p align="center"><img src="../brand/pixelgo-banner.svg" width="680" alt="PixelGo official pastel identity" /></p>

# Deployment

PIXEL GO separates **building a release** from **authorizing a deployment**.

A Git tag matching `v*` runs `.github/workflows/release.yml`.

## Release pipeline

~~~text
v1.2.3 tag
    ↓
┌─────────────────────────────────────────────┐
│                                             │
Backend image        Android AAB      iOS archive
    ↓                    ↓                ↓
GHCR + SBOM           artifact         artifact
+ provenance
    │
    ↓
staging environment
    ↓
/ready smoke test
    ↓
production environment approval
    ↓
candidate container on :18080
    ↓
local /ready preflight
    ↓
promote validated image
    ↓
external /ready smoke test
~~~

The backend image is published as:

~~~text
ghcr.io/<owner>/<repo>:<git-tag>
ghcr.io/<owner>/<repo>:sha-<commit-sha>
~~~

BuildKit publishes SBOM and provenance attestations with the image.

## Why deployment is optional

The repository cannot safely contain a real server, SSH key, APNs key, FCM service account, database password or store-signing credential.

Therefore release artifacts always build, while staging/production deployment is enabled only when the corresponding GitHub repository/environment variables and secrets are configured.

No fake production credential is committed to make the workflow look complete.

## Staging GitHub Environment

Create a GitHub Environment named `staging`.

Repository/environment variables:

~~~text
STAGING_DEPLOY_ENABLED=true
STAGING_APP_URL=https://staging.example.com
STAGING_HEALTH_URL=https://staging.example.com
~~~

Environment secrets:

~~~text
STAGING_SSH_HOST
STAGING_SSH_USER
STAGING_SSH_PRIVATE_KEY
STAGING_SSH_KNOWN_HOSTS
~~~

The staging host must:

- have Docker installed;
- already be authenticated to GHCR with a read-only package credential;
- expose the application through the desired TLS ingress/reverse proxy;
- contain `/etc/pixelgo/staging.env` readable only by the deployment account.

The workflow replaces `pixelgo-api-staging` and then polls:

~~~text
GET /ready
~~~

Deployment fails unless readiness becomes healthy.

## Production GitHub Environment

Create a GitHub Environment named `production`.

Configure **required reviewers** in GitHub for this environment. That gives the production promotion an explicit human approval gate without hard-coding approval logic into the application.

Variables:

~~~text
PRODUCTION_DEPLOY_ENABLED=true
PRODUCTION_APP_URL=https://api.example.com
PRODUCTION_HEALTH_URL=https://api.example.com
~~~

Secrets:

~~~text
PRODUCTION_SSH_HOST
PRODUCTION_SSH_USER
PRODUCTION_SSH_PRIVATE_KEY
PRODUCTION_SSH_KNOWN_HOSTS
~~~

The production host must have `/etc/pixelgo/production.env` and read-only GHCR authentication.

## Production preflight

The workflow does not immediately replace the live container.

It first starts the exact release image as:

~~~text
pixelgo-api-candidate
127.0.0.1:18080 → container :8080
~~~

It polls the candidate's `/ready` endpoint from the production host. Only a healthy candidate is promoted to the live `pixelgo-api` container.

This catches configuration failures such as:

- PostgreSQL unavailable;
- Redis unavailable;
- object storage unavailable;
- invalid startup configuration;
- missing migrations.

After promotion, the workflow performs an external readiness smoke test through the deployed URL.

## Host environment files

Deployment environment files stay on the host and outside source control.

A production file typically provides:

~~~text
PIXELGO_REQUIRE_AUTH=true
PIXELGO_JWT_SECRET=...
DATABASE_URL=...
REDIS_URL=...

OBJECT_STORAGE_ENDPOINT=...
OBJECT_STORAGE_BUCKET=...
OBJECT_STORAGE_ACCESS_KEY=...
OBJECT_STORAGE_SECRET_KEY=...
OBJECT_STORAGE_REGION=...
OBJECT_STORAGE_PREFIX=transfers

APNS_KEY_ID=...
APNS_TEAM_ID=...
APNS_TOPIC=com.armaabetancourtt.pixelgo
APNS_PRIVATE_KEY_B64=...

FCM_SERVICE_ACCOUNT_JSON_B64=...
~~~

Do not copy the development values from `server/.env.example` into production.

## GHCR host authentication

Deployment hosts should be authenticated to GHCR ahead of time with a **read-only** package credential.

The workflow intentionally does not transmit a package token over SSH on every release.

## Rollback

Images are immutable and tagged by both release tag and commit SHA. A rollback is therefore an image promotion, not a rebuild.

On the host:

~~~bash
docker pull ghcr.io/<owner>/<repo>:v1.2.2
docker rm -f pixelgo-api || true
docker run -d \
  --restart unless-stopped \
  --name pixelgo-api \
  --env-file /etc/pixelgo/production.env \
  -p 8080:8080 \
  ghcr.io/<owner>/<repo>:v1.2.2
~~~

Then verify `/ready`.

A production platform with native blue/green or rolling deployments can replace the SSH/Docker steps while keeping the same image, readiness endpoint and environment gates.

## Mobile distribution

The release workflow currently produces:

- Android release AAB;
- unsigned iOS archive.

Play Internal Testing and TestFlight upload remain credentialed deployment steps. They are intentionally not fabricated in CI until real store applications and signing credentials exist.
