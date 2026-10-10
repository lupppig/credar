# ADR 0001: Repository layout and application toolchains

- Status: Accepted
- Date: 2026-09-21

## Context

The platform contains a Go backend, a Next.js frontend, shared public
contracts, database migrations, and a reproducible local stack. Later phases
will change these parts together, so contributors need one unambiguous place
for each artifact and reproducible toolchains.

## Decision

Use a monorepo with these top-level ownership boundaries:

- `backend/` contains one Go module at
  `github.com/lupppig/credar/backend`. It builds the modular-monolith API and
  its background-worker entry points.
- `frontend/` contains one Next.js application using the App Router and strict
  TypeScript.
- `contracts/` contains source API and event schemas shared across application
  boundaries.
- `deploy/` contains Docker Compose and local dependency configuration.
- `docs/` contains ADRs and engineering documentation.
- `scripts/` contains stable command implementations used locally and in CI.

Pin these initial toolchains:

- Go 1.27.2.
- Node.js 24.21.0 LTS.
- Next.js 16.3.3 Active LTS.
- TypeScript 6.0.x, pinned to an exact resolved version in the lockfile.
- pnpm 12.5.1, declared through the root `packageManager` field.

The root exposes repository-wide commands. Go dependency versions are locked
by `go.mod` and `go.sum`; JavaScript dependency versions are locked by
`pnpm-lock.yaml`. CI and container builds use the same declared major/minor
toolchains and pinned patch releases.

## Alternatives considered

- Separate repositories would allow independent release cadence but would make
  atomic API/schema changes and local integration work harder at the current
  team and product size.
- Multiple Go modules would increase versioning and dependency overhead before
  a real independent boundary exists.
- npm and Yarn are capable choices, but pnpm provides strict dependency
  isolation and efficient reproducible workspace installs.

## Consequences

Cross-application contracts can land atomically. Backend packages must still
preserve domain boundaries; sharing a repository does not permit frontend or
infrastructure dependencies to leak into the domain. A toolchain upgrade is an
explicit repository change with build and compatibility evidence.
