# Phase 02 handoff — Identity, tenancy, and RBAC

## Delivered

- Kratos validates credentials and maps identities to application users;
  PostgreSQL remains authoritative for tenants, memberships, roles, permissions,
  assignments, and audit history.
- Every tenant endpoint requires an authenticated user and explicit canonical
  `X-Tenant-ID`. Selection never grants access; active membership, permission,
  and tenant-scoped repository predicates are evaluated server-side.
- Migration `000006_identity_rbac` seeds the closed permission catalogue and
  deterministic immutable roles: `PLATFORM_ADMIN`, `TENANT_OWNER`,
  `TENANT_ADMIN`, `TENANT_AUDITOR`, and `TENANT_MEMBER`.
- Database triggers reject platform roles on memberships, tenant roles on
  platform users, cross-tenant custom-role assignments, and role/permission
  scope mismatches.
- Tenant creation assigns `TENANT_OWNER` in the same transaction. Membership
  creation assigns its requested roles atomically. Last-active-owner and
  last-active-platform-administrator invariants are locked and enforced.
- Custom role creation and updates accept only tenant permissions. System roles
  are immutable, and assigned custom roles cannot be deleted.
- Protected APIs now cover tenants, memberships, membership roles, custom roles,
  platform-role replacement, and immutable tenant audit history.
- `cmd/bootstrap-admin` resolves a Kratos identity through the admin API and
  transactionally creates/maps the application user and assigns the first
  `PLATFORM_ADMIN`. It refuses to run after any platform assignment exists and
  records a `SYSTEM` audit actor.
- The Next.js identity console proxies credentials server-side and provides
  Kratos sign-in, recovery, and verification entry points; tenant switching;
  team lifecycle actions; role presentation and creation; a permission matrix;
  audit history; and explanatory disabled/denied states.

## Security and isolation evidence

- PostgreSQL integration tests cover system seeding, one-time bootstrap,
  platform and tenant permission evaluation, custom-role escalation denial,
  missing permission, tenant substitution, owner transfer, and last-owner
  denial.
- `authorizationtest` exposes reusable allowed, permission-denied, and hidden-
  tenant assertions for later tenant-owned capabilities.
- The real Kratos journey signs in, bootstraps an administrator, creates and
  selects two tenants, performs an allowed administration request, adds a
  tenant member, proves that member's administration request is denied by the
  backend, rejects substituted tenant context, and switches the administrator's
  active tenant.
- Audit and assignment events are inserted in the same transaction as their
  state changes. Event payloads conform to the frozen Phase 02 schemas and omit
  credentials, session tokens, and provider payloads.

## Operator bootstrap

Set host-reachable `CREDAR_DATABASE_URL` and `CREDAR_KRATOS_ADMIN_URL`, then run:

```text
make bootstrap-admin KRATOS_IDENTITY_ID=<kratos-identity-uuid>
```

The command is intentionally one-time. Subsequent platform-administrator
assignments use the authenticated platform-role API, which preserves at least
one active administrator.

## Acceptance commands

- `make format-check`
- `make lint`
- `make contracts`
- `make migrations-check`
- `go test -race ./...`
- `make database-test`
- `make authentication-test`
- `make full-stack-test`
- `make e2e`

## Next phase

Phase 03 can consume the authenticated user, explicit tenant context, and
centralized permission evaluator without introducing its own authorization
bypass or tenant-selection semantics.
