# P02-03 handoff — Tenant and membership domain

## Delivered

- Migration `000005_tenancy_memberships` adds tenant and membership tables with
  tenant-scoped foreign keys, lifecycle timestamp constraints, one-current-
  membership uniqueness, stable page indexes, and durable idempotency records.
- Tenant creation stores the tenant, initial active membership, two immutable
  audit records, two outbox events, and the replayable response in one
  transaction.
- Tenant creation idempotency is scoped to actor, operation, and key. Matching
  retries return the original result, conflicting payloads fail with
  `idempotency_key_reused`, and concurrent duplicates serialize behind a
  transaction advisory lock.
- Membership transitions allow `ACTIVE` to `DISABLED` or `REMOVED`, and
  `DISABLED` to `ACTIVE` or `REMOVED`. Removal is terminal; rejoining creates a
  new membership identity while preserving the removed history.
- `GET /v1/session/context` now returns all active memberships. An optional
  canonical `X-Tenant-ID` is echoed only when the authenticated user has an
  active membership in an active tenant.
- Validated tenant IDs enter the standard request context and structured HTTP
  logs alongside user, request, and correlation IDs.

## Authorization boundary

Tenant and membership writes are available only through an application service
that requires `platform.tenants:create` or `memberships:manage` from an
authorization port before persistence is called. The production HTTP router
does not register management routes yet: P02-04 must provide the concrete role
and permission evaluator first.

Tenant creation currently produces the initial active membership but does not
claim an owner role. P02-04 must seed `TENANT_OWNER` and attach it in the same
creation transaction before enabling the public mutation route. This preserves
the contract's prohibition on authentication-only privilege and avoids a
temporary permissive implementation.

## Cross-cutting review

- Security and tenant isolation: session tenant selection checks active
  membership server-side; malformed IDs return 400 and absent, disabled, or
  removed memberships return 403. Membership mutation lookup includes both the
  membership and tenant IDs so cross-tenant substitution is indistinguishable
  from absence.
- Idempotency and concurrency: PostgreSQL records retain canonical request
  hashes and outcomes for at least 24 hours. Advisory locks and partial unique
  constraints cover concurrent tenant creation and membership insertion.
- Audit and events: `tenant.created`, `membership.created`, and
  `membership.status_changed` audit entries share the state transaction.
  `tenant.created` and `membership.status_changed` integration events use the
  existing v1 schemas and omit reasons and identity-provider data.
- Failures: persistence and event failures roll back the complete operation.
  Tenant-context availability failures are closed with a stable 503 problem.
- Money and ledger: no monetary or ledger state is introduced by this task.
- Roles: role assignment, last-owner enforcement, permission evaluation, and
  management-route activation remain together in P02-04.

## Acceptance evidence

- Unit tests cover status-transition rules, permission-gated service calls,
  actor-context binding, session responses, and stable problem mappings.
- PostgreSQL tests cover migration forward/rollback/re-apply, matching and
  conflicting idempotency retries, concurrent duplicates, lifecycle changes,
  terminal removal, rejoin, cross-tenant denial, atomic rollback, audit, and
  outbox counts.
- The real Kratos harness proves authenticated tenant selection, malformed
  context rejection, and inactive-membership denial through the HTTP handler.

Commands:

- `go test -race ./...`
- `make database-test`
- `make authentication-test`
- `make full-stack-test`

## Next task

**P02-04 — Implement roles, permissions, and authorization**: seed immutable
system roles, implement custom-role scope rules, assign roles to memberships,
enforce last-owner and last-platform-admin invariants, provide the concrete
authorization evaluator, and activate the protected tenant/membership routes.
