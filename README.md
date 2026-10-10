# Credar

Credar is the implementation of a multi-tenant B2B credit infrastructure
platform. Businesses can borrow, provide financing to their customers, or do
both. BNPL is one financing product built on the platform rather than the
platform's entire domain model.

## Delivery status

The dependency-ordered roadmap starts with Phase 00. Foundation and local
runtime acceptance are complete, so Phase 02 can now establish authenticated,
tenant-scoped authorization.

| Phase                                       | Status   | Outcome                                              |
| ------------------------------------------- | -------- | ---------------------------------------------------- |
| 00 — Foundation and production CI           | Complete | Repository contracts and repeatable quality pipeline |
| 01 — Local runtime and infrastructure spine | Complete | Healthy local stack and transaction-to-event proof   |
| 02 — Identity, tenancy, and RBAC            | Active   | Tenant and membership domain implemented             |

P00-01 through P00-06 are complete. Foundational decisions and repository
commands are recorded, both applications and production images build and pass
process-level smoke checks, shared transport contracts are machine-readable,
and CI enforces the same quality gates. See the
[Phase 00 handoff](docs/handoffs/phase-00.md) for acceptance evidence. P01-01
through P01-07 are also complete: pinned local infrastructure starts from empty
volumes; the Go process has validated lifecycle and health behavior; PostgreSQL
has an explicit migration, pooling, readiness, and transaction foundation;
committed integration events flow through a transactional outbox into JetStream
with leased retries and idempotent consumption; Redis and SeaweedFS sit behind
readiness-aware, isolated application adapters; and the Next.js enterprise
shell centralizes validated environment and correlated backend access. One
isolated command now builds the complete stack, waits on application and
dependency health, and proves the correlated transaction-to-event path. See the
[Phase 01 handoff](docs/handoffs/phase-01.md). P02-01 froze the application
identity mapping, explicit tenant context, membership lifecycle, system and
custom roles, permission catalogue, audit semantics, APIs, and internal event
payloads in [the identity contract](docs/identity-authorization.md). P02-02
validates Kratos sessions server-side and maps them exactly once to application
users with transactional audit and outbox records. P02-03 adds idempotent tenant
creation, membership lifecycle persistence, and explicit active-tenant session
context. The next task is **P02-04 — Implement roles, permissions, and
authorization**.
Accepted decisions are recorded in
[`docs/adr`](docs/adr/README.md).

## Intended repository layout

```text
backend/       Go modular monolith and background workers
frontend/      Next.js enterprise and developer application
contracts/     OpenAPI and versioned event schemas
deploy/        Docker Compose and local service configuration
docs/          Architecture decisions and engineering documentation
scripts/       Stable, non-interactive repository commands
```

Only directories with implemented content are added to version control.

## Repository commands

Run `make help` for the command list and `make doctor` to validate local
toolchains. The stable command surface is:

| Command                                 | Purpose                                          |
| --------------------------------------- | ------------------------------------------------ |
| `make bootstrap`                        | Install locked dependencies                      |
| `make format` / `make format-check`     | Apply or verify formatting                       |
| `make lint`                             | Run static analysis                              |
| `make contracts`                        | Validate OpenAPI and event/audit schemas         |
| `make migrations-check`                 | Validate migration naming and up/down pairing    |
| `make unit`                             | Run deterministic unit tests                     |
| `make integration`                      | Run tests against real infrastructure boundaries |
| `make authentication-test`              | Verify Kratos sessions and application mapping   |
| `make infrastructure-test`              | Verify dependencies from isolated empty volumes  |
| `make full-stack-test`                  | Prove the built stack and correlated event path  |
| `make event-test`                       | Prove transactional event delivery and deduping  |
| `make adapters-test`                    | Verify Redis and object-storage adapter behavior |
| `make e2e`                              | Run critical browser journeys                    |
| `make build`                            | Build production application artifacts           |
| `make smoke`                            | Start and probe both built applications          |
| `make generate` / `make generate-check` | Refresh or verify derived source                 |
| `make migrate-up` / `make migrate-down` | Apply or revert local migrations                 |
| `make infra-up` / `make infra-down`     | Start or stop local dependency services          |
| `make infra-reset`                      | Delete local dependency services and their data  |
| `make dev` / `make down`                | Start or stop the complete local stack           |
| `make check`                            | Run the fast local quality gate                  |
| `make ci`                               | Run the non-browser CI gate                      |

Configuration and generated-source rules are documented in
[`docs/configuration.md`](docs/configuration.md) and
[`docs/generated-files.md`](docs/generated-files.md).
Local service endpoints, persistence, and reset behavior are documented in
[`docs/local-infrastructure.md`](docs/local-infrastructure.md).

## Architectural boundaries

- Go remains one deployable modular monolith; NATS does not imply
  microservices.
- Gin is the HTTP transport adapter; domain and application packages do not
  depend on Gin contexts or types.
- PostgreSQL is authoritative transactional state.
- NATS JetStream carries committed asynchronous events through an outbox.
- Redis is limited to ephemeral coordination and caching.
- SeaweedFS stores object bytes behind an application-owned adapter.
- ORY Kratos authenticates identities; the application owns tenants,
  memberships, roles, permissions, and resource authorization.
- External financial, credit, identity, and banking providers begin as
  deterministic mocks behind replaceable interfaces.
- Tenant isolation, authorization, auditability, idempotency, immutable
  financial history, and balanced ledger entries are system invariants.

Production cloud infrastructure and deployment automation are intentionally
out of scope.
