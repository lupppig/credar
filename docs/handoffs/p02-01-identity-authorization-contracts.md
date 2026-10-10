# P02-01 handoff — Identity and authorization contracts

## Outcome

Credar now has one accepted contract for Kratos identity mapping, explicit
tenant context, membership lifecycle, platform and tenant roles, custom-role
limits, permission evaluation, audit history, API errors, HTTP operations, and
internal events. No runtime authorization behavior is claimed yet.

## Contracts

- Added `docs/identity-authorization.md` and ADR 0006.
- Expanded OpenAPI to version 0.2.0 with session context, platform-role,
  tenant, membership, tenant-role, and audit operations.
- Added Kratos cookie and session-token security schemes and stable identity/
  authorization schemas and errors.
- Added versioned JSON Schemas for `identity.user_linked`,
  `identity.platform_roles_changed`, `tenant.created`,
  `membership.status_changed`, `membership.roles_changed`, and `role.changed`.
- No migration or generated source changed. Runtime implementations must not
  advertise these endpoints until their owning P02 tasks land.

## Decisions

- Kratos owns authentication only; PostgreSQL-owned Credar models determine
  authorization.
- `X-Tenant-ID` is request context and is validated on every protected use
  case. Active tenant is not persisted globally.
- Platform roles attach to users; tenant roles attach to memberships.
- Initial platform privilege comes only from a one-time operator command. A
  first-login privilege shortcut is prohibited.
- System roles are immutable. Tenant custom roles cannot contain platform
  permissions or cross tenant boundaries.
- Cross-tenant resource lookup returns the same result as absence.

## Cross-cutting review

- Tenant isolation and authorization: directly defined and required at query,
  application-policy, and transport boundaries.
- Audit history: consequential changes are transactional and append-only.
- Ledger and financial balances: not affected.
- Idempotency and concurrency: tenant creation declares an idempotency key;
  last-owner and last-platform-admin invariants require transactional locking.
- Events and API compatibility: new pre-implementation v1 contracts; no
  existing product endpoint or published event changes meaning.
- Historical data and migrations: no persistence change in this task.
- Sandbox behavior: one-time bootstrap is allowed only as an explicit operator
  flow; no automatic privilege grant.
- Observability: authenticated user and tenant IDs join existing request and
  correlation IDs; secrets and identity-provider payloads remain excluded.

## Verification

- `make check`
- `git diff --check`

## Risks and deferred work

Invitation delivery, enterprise SSO, API keys, fine-grained resource roles,
customer identity/KYC, production identity migration, and public identity
webhooks remain out of scope.

## Next task

**P02-02 — Integrate Kratos authentication**.
