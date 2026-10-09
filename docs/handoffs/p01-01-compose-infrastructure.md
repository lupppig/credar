# P01-01 handoff — Compose infrastructure

## Outcome

The local PostgreSQL, NATS JetStream, Redis, SeaweedFS, and ORY Kratos services
are defined with exact image versions and immutable multi-platform digests.
Named storage, loopback-only host ports, health checks, readiness-aware startup,
and reproducible empty-volume verification are in place.

## Decisions

- One project-scoped bridge network isolates the dependency stack while keeping
  service discovery explicit.
- PostgreSQL initializes separate `credar` and `kratos` databases. Kratos
  migrations run as a one-shot dependency before the server becomes eligible
  to start.
- NATS starts with JetStream enabled and persists its store. Redis retains local
  data across ordinary restarts but remains semantically non-authoritative.
- SeaweedFS uses its single-node development server with authenticated S3 and a
  named volume. Object adapter behavior and bucket lifecycle remain P01-05.
- Host ports bind only to `127.0.0.1` and are individually overridable through
  documented environment variables.

## Contracts and migrations

- No application API, event contract, domain table, or application migration
  changed.
- A PostgreSQL initialization script creates the infrastructure-owned Kratos
  database. Kratos applies its own vendor migrations through its pinned image.
- The minimal identity schema only makes Kratos bootable; application identity
  mapping and user flows remain Phase 02 work.

## Verification

- `docker compose config --quiet` validates the resolved Compose model.
- `make infrastructure-test` starts a unique project from empty volumes and
  verifies all five dependencies before removing the project and its data.
- The test proves JetStream-specific health, Redis PING, both PostgreSQL
  databases, the SeaweedFS S3 listener, successful Kratos migration, and Kratos
  admin readiness.
- CI runs the same integration harness and retains sanitized service diagnostics
  only when it fails.

## Remaining risks and next task

This is a single-node development topology, not a production availability or
backup design. Application processes do not yet validate configuration or
reflect dependency readiness. Self-service UI routes are placeholders until
the frontend runtime shell exists.

Next: **P01-02 — Bootstrap the Go runtime**.
