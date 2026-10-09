# Configuration contract

Configuration is read from environment variables at process startup and
validated before the process accepts work. Missing, malformed, conflicting, or
unsafe values produce an actionable startup error naming the variable. Runtime
code does not read configuration opportunistically after startup.

## Naming

- Backend and server-only frontend values use the `CREDAR_` prefix.
- Browser-visible values use the `NEXT_PUBLIC_CREDAR_` prefix.
- A value with the `NEXT_PUBLIC_` prefix is public by definition and must never
  contain credentials, signing material, personal data, or internal-only
  endpoints.
- Variables use uppercase snake case and include a unit in the name when the
  value is not self-describing, such as `_SECONDS` or `_BYTES`.

`.env.example` documents a complete non-secret local configuration. Developers
copy it to the ignored `.env` file. The values in `.env.example` are local-only
defaults and must never be reused as production credentials.

## Secrets

- Real secrets are never committed, placed in `NEXT_PUBLIC_` variables, logged,
  included in error messages, or uploaded as CI artifacts.
- CI secrets are scoped to the smallest job that consumes them. The Phase 00 CI
  baseline does not require production or deployment credentials.
- Application configuration types must redact sensitive fields when formatted.
- Secret rotation must not require recompiling either application.

## Environment identity

`CREDAR_ENVIRONMENT` identifies the runtime safety boundary. Initial accepted
values are `local`, `test`, and `sandbox`. No implicit `production` behavior is
introduced during the local-platform roadmap. Sandbox controls must later
verify this value and their persisted environment scope; checking an
environment variable alone is not sufficient isolation.

## API runtime

- `CREDAR_LOG_LEVEL` accepts the structured logger levels `debug`, `info`,
  `warn`, or `error`.
- `CREDAR_HTTP_ADDRESS` is a `host:port` listen address. An empty host binds
  all container interfaces.
- `CREDAR_HTTP_SHUTDOWN_TIMEOUT_SECONDS` bounds graceful HTTP and dependency
  shutdown.
- `CREDAR_READINESS_TIMEOUT_SECONDS` bounds each aggregate dependency
  readiness evaluation.

Timeout values are whole seconds from 1 through 60. Invalid startup
configuration names the affected variable and stops the process before it
accepts work.

## PostgreSQL

- `CREDAR_DATABASE_URL` is the PostgreSQL connection string. It is treated as
  sensitive because it may contain credentials and is never included in logs
  or formatted configuration output.
- `CREDAR_DATABASE_MAX_CONNECTIONS` bounds the process-wide pool from 1 to
  100 connections.
- `CREDAR_DATABASE_MIN_CONNECTIONS` keeps a baseline of warm connections and
  must be between zero and the configured maximum.

The API creates the pool during startup but reports connectivity through
`/readyz`. A temporary PostgreSQL outage therefore removes the process from
readiness without failing liveness or triggering a restart loop.

## NATS and outbox relay

- `CREDAR_NATS_URL` is treated as sensitive because it may embed
  credentials.
- `CREDAR_OUTBOX_POLL_MILLISECONDS`, `CREDAR_OUTBOX_LEASE_SECONDS`, and
  `CREDAR_OUTBOX_BATCH_SIZE` bound relay work and abandoned-lease recovery.
- `CREDAR_OUTBOX_MAX_ATTEMPTS`,
  `CREDAR_OUTBOX_BASE_BACKOFF_SECONDS`, and
  `CREDAR_OUTBOX_MAX_BACKOFF_SECONDS` define bounded exponential retry and
  terminal dead-letter behavior.

The relay never holds a PostgreSQL transaction open while publishing to NATS.
JetStream and durable-consumer health contribute to `/readyz` independently.

## Redis

- `CREDAR_REDIS_URL` is treated as sensitive because it may contain
  credentials. It accepts `redis` and `rediss` URLs.
- `CREDAR_REDIS_KEY_PREFIX` isolates all keys owned by one runtime or test.

The platform adapter accepts only values with a positive expiry. Redis remains
ephemeral and is never a source of truth for financial or historical state.

## Object storage

- `CREDAR_OBJECT_STORAGE_ENDPOINT` is the SeaweedFS S3-compatible HTTP or
  HTTPS endpoint and must not contain credentials or a path.
- `CREDAR_OBJECT_STORAGE_BUCKET` is the adapter-owned bucket, created during
  dependency startup when it does not exist.
- `CREDAR_OBJECT_STORAGE_ACCESS_KEY` and
  `CREDAR_OBJECT_STORAGE_SECRET_KEY` are sensitive server-only credentials.

The adapter uses opaque UUIDv7 object keys. Business content-type, size,
authorization, retention, and metadata rules remain with the future module
that owns each object use case.

## Next.js runtime

- `NEXT_PUBLIC_CREDAR_API_BASE_URL` is the browser-visible API origin. It is
  embedded during the production build and must contain no credentials, path,
  query, or fragment.
- `CREDAR_BACKEND_INTERNAL_URL` is the server-only API origin used by the
  centralized backend client. It is validated when that server boundary is
  constructed and is never embedded into browser output.

Both accept only absolute HTTP or HTTPS origins and have explicit local
defaults. Backend requests create or propagate a canonical UUIDv7
`X-Correlation-ID`; response request/correlation identifiers are retained in
the normalized client error for operational support.

## Compose host ports

`CREDAR_BACKEND_HOST_PORT` and `CREDAR_FRONTEND_HOST_PORT` override the
loopback ports published by Docker Compose. They default to `8080` and `3000`.
The existing dependency-specific `_HOST_PORT` values follow the same rule and
never change container-to-container service addresses.

The frontend image consumes `NEXT_PUBLIC_CREDAR_API_BASE_URL` as a build
argument because public Next.js values are embedded in browser assets. The
full-stack harness discovers its Docker-assigned backend port before building
the frontend, so the isolated artifact contains the exact test origin.

## Rename compatibility

The Credar rename changes the environment-variable prefix, Compose project,
PostgreSQL database and role, object-storage bucket, Redis key prefix, NATS
stream and subjects, and application binary names. Existing local Compose
volumes were initialized with the former database credentials and cannot be
reused as-is. Run `make infra-reset` once after updating, then start the stack
again with `make infra-up`.

Migration `000003_rename_platform_namespace` preserves database migration
history while translating persisted outbox subjects and the infrastructure
proof consumer name. NATS and the other local infrastructure stores are
development-only at this stage and are recreated by the reset; no production
namespace migration is implied by this local procedure.
