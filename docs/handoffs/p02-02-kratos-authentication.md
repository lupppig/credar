# P02-02 handoff — Kratos authentication

## Delivered

- The API validates browser cookies and native `X-Session-Token` credentials
  through Kratos `sessions/whoami`; it does not store credentials, recovery
  material, verification state, or session payloads.
- A valid Kratos identity resolves to one application-owned user. The first
  successful session creates the UUIDv7 user, immutable
  `identity.user_linked` audit record, and matching outbox event in one
  PostgreSQL transaction.
- Advisory transaction locks serialize concurrent first-login requests by
  Kratos identity and normalized email. Repeated sessions are idempotent, while
  identity/email collisions deny access with `identity_mapping_conflict`.
- `GET /v1/session/context` returns the authenticated user with empty
  memberships and no active tenant until P02-03 supplies that domain state.
- Disabled application users are denied independently of an otherwise valid
  Kratos session.
- Kratos public/admin endpoints and request timeouts are validated at startup;
  Kratos readiness participates in `/readyz`.

## Persistence and contracts

Migration `000004_identity_authentication` owns the new `identity` schema:

- `identity.users` stores the unique Kratos identity link, normalized email
  snapshot, application status, and timestamps.
- `identity.audit_events` stores allow-listed audit metadata and rejects update
  or delete operations through database triggers.
- The existing `platform.outbox_events` table carries the versioned
  `identity.user_linked` event without email, Kratos identifiers, or session
  data.

Authentication failures use `application/problem+json` with request and
correlation IDs. Missing and invalid/expired sessions return 401, disabled
users return 403, mapping conflicts return 409, and an unavailable Kratos or
database boundary returns 503.

## Cross-cutting review

- Security: credentials are forwarded only to Kratos and never logged or
  persisted. Application disablement remains authoritative for protected API
  access.
- Tenancy: no tenant is inferred from identity or email. Memberships and
  explicit tenant context remain P02-03 work.
- Money and ledger: not applicable to this identity task; no monetary state or
  ledger path was introduced.
- Idempotency and concurrency: transaction-scoped advisory locks and unique
  constraints prevent duplicate or conflicting user links.
- Events and audit: the user, audit record, and outbox event share one commit;
  an outbox failure rolls the entire mapping back.
- Observability and failures: request/correlation IDs cross the HTTP,
  persistence, audit, and event boundaries; authenticated user IDs enter
  request logs; dependency failures fail closed with a stable 503 problem.
- Migrations: forward, full rollback, and re-apply are exercised against an
  empty PostgreSQL volume.

## Acceptance evidence

- `go test ./...`
- `make database-test`
- `make authentication-test`

The authentication harness uses the pinned Kratos image to create an identity,
perform a native password login, validate the issued token, reject a bad token,
observe actual session expiry, and deny a forced application mapping conflict.
Repository integration tests also prove concurrent mapping and atomic rollback.

## Next task

**P02-03 — Implement tenant and membership domain**: add tenant creation,
membership lifecycle and constraints, explicit tenant selection, tenant-scoped
audit records, and session-context memberships. P02-04 remains responsible for
roles, permissions, and authorization policy evaluation.
