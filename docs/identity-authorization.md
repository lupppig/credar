# Identity, tenancy, and authorization contract

This contract defines the Phase 02 security boundary. Kratos authenticates a
human identity. Credar maps that identity to an application user and owns every
tenant, membership, role, permission, resource-scope, and audit decision.

## Identity mapping

- A platform user has a UUIDv7 `id`, one unique Kratos identity ID, a normalized
  email snapshot, `ACTIVE` or `DISABLED` status, and creation/update timestamps.
- Kratos remains authoritative for credentials, recovery, verification, and
  authentication sessions. Credar never stores password or recovery material.
- The email trait is copied only for display and controlled lookup. It never
  grants membership or authorization.
- A valid Kratos session whose identity is not mapped creates the application
  user exactly once. A Kratos identity or normalized email mapped to conflicting
  users returns `identity_mapping_conflict` and grants no access.
- Disabling a platform user immediately prevents all protected access without
  deleting memberships or audit history.

## Tenant context

Protected tenant endpoints require `X-Tenant-ID`. The value selects context;
it never grants access. For every request the server verifies the authenticated
user, an active membership, permission, and resource ownership. Repository
queries include tenant scope rather than filtering results in memory.

Tenant switching is client-side context selection. Credar does not persist one
globally active tenant because concurrent browser tabs and API clients may act
in different authorized tenants. `GET /v1/session/context` returns the current
user and available active memberships; the selected tenant is echoed only when
the request supplies `X-Tenant-ID`.

Cross-tenant resource identifiers return `resource_not_found` so resource
existence is not disclosed. A valid tenant context with an inactive membership
or missing permission returns a forbidden problem.

## Membership lifecycle

Membership statuses are:

- `ACTIVE`: may authorize requests through assigned roles.
- `DISABLED`: temporarily denies access while preserving assignments.
- `REMOVED`: terminal historical record; it never authorizes access.

A user has at most one non-removed membership in a tenant. Rejoining after
removal creates a new membership ID. Status transitions are `ACTIVE` to
`DISABLED` or `REMOVED`, and `DISABLED` to `ACTIVE` or `REMOVED`. A removed
membership cannot transition. A tenant must retain at least one active owner;
an operation violating this invariant returns `last_tenant_owner_required`.

Phase 02 membership creation accepts an existing application `user_id`.
Invitation delivery and user-directory discovery are separate future product
decisions and are not implied by this contract.

## Roles and permissions

Roles have `PLATFORM` or `TENANT` scope. System roles are immutable and seeded
deterministically. Tenant custom roles belong to exactly one tenant, have unique
case-insensitive names within that tenant, and may contain only tenant-scoped
permissions from the closed catalogue below.

Platform roles are assigned directly to application users. Tenant roles are
assigned only through memberships. These assignment paths never overlap.

| Permission                    | Scope    | Meaning                                      |
| ----------------------------- | -------- | -------------------------------------------- |
| `platform.tenants:create`     | Platform | Create a tenant and its initial owner        |
| `platform.tenants:read`       | Platform | Inspect tenants across the platform          |
| `platform.memberships:manage` | Platform | Perform platform-authorized membership work  |
| `tenants:read`                | Tenant   | Read the current tenant                      |
| `tenants:manage`              | Tenant   | Update allowed tenant settings               |
| `memberships:read`            | Tenant   | List and inspect tenant memberships          |
| `memberships:manage`          | Tenant   | Add, disable, remove, and reactivate members |
| `roles:read`                  | Tenant   | Read roles and permission assignments        |
| `roles:manage`                | Tenant   | Create and modify tenant custom roles        |
| `audit:read`                  | Tenant   | Read the tenant audit history                |

Initial system roles are:

- `PLATFORM_ADMIN` (`PLATFORM`): all three `platform.*` permissions. Assignment
  is available only to an existing platform administrator through an explicit
  platform-authorized flow.
- `TENANT_OWNER` (`TENANT`): every tenant permission. Only an owner or platform
  administrator may assign it, and the last active owner cannot be disabled,
  removed, or stripped of the role.
- `TENANT_ADMIN` (`TENANT`): every tenant permission except `tenants:manage`.
- `TENANT_AUDITOR` (`TENANT`): `tenants:read`, `memberships:read`, `roles:read`,
  and `audit:read`.
- `TENANT_MEMBER` (`TENANT`): `tenants:read` only.

Role assignment never widens resource scope. Tenant administrators cannot
assign platform roles, add platform permissions to custom roles, mutate system
roles, or operate on another tenant.

The first platform administrator is created by an explicit, one-time operator
bootstrap command that accepts a Kratos identity ID, resolves it through the
Kratos admin API, creates or maps the application user, and assigns
`PLATFORM_ADMIN` transactionally. The command refuses to run after any platform
role assignment exists and records a `SYSTEM` audit actor. There is no
"first authenticated user wins" behavior. Later platform-role assignments use
the authenticated platform API. The platform must retain at least one active
platform administrator.

## Audit semantics

Identity, membership, role, and permission changes append an audit record in
the same PostgreSQL transaction as the state change. Records are immutable and
contain action, actor, tenant when applicable, target type/ID, request and
correlation IDs, source, reason when supplied, and a small allow-listed change
summary. Credentials, session tokens, arbitrary request bodies, and Kratos
payloads are never recorded.

Stable actions are `identity.user_linked`, `identity.user_disabled`,
`identity.platform_roles_changed`, `tenant.created`, `membership.created`,
`membership.status_changed`, `membership.roles_changed`, `role.created`,
`role.updated`, and `role.deleted`.

## API errors

All errors use the shared `application/problem+json` schema.

| HTTP | Code                            | Meaning                                        |
| ---- | ------------------------------- | ---------------------------------------------- |
| 400  | `tenant_context_required`       | A protected tenant endpoint lacks the header   |
| 400  | `invalid_tenant_context`        | `X-Tenant-ID` is malformed                     |
| 401  | `authentication_required`       | No Kratos session was supplied                 |
| 401  | `invalid_session`               | The supplied session is invalid or expired     |
| 403  | `user_disabled`                 | The mapped platform user is disabled           |
| 403  | `membership_inactive`           | Membership is disabled or removed              |
| 403  | `permission_denied`             | Permission evaluation denied the action        |
| 404  | `resource_not_found`            | Resource is absent or outside tenant scope     |
| 409  | `identity_mapping_conflict`     | Kratos identity mapping is inconsistent        |
| 409  | `idempotency_key_reused`        | The key was reused with different input        |
| 409  | `membership_already_exists`     | A current membership already exists            |
| 409  | `invalid_membership_transition` | The requested status transition is forbidden   |
| 409  | `last_tenant_owner_required`    | The change would leave no active owner         |
| 409  | `last_platform_admin_required`  | The change would leave no platform admin       |
| 409  | `system_role_immutable`         | A system role mutation was attempted           |
| 409  | `permission_scope_escalation`   | A custom role requested forbidden permission   |
| 409  | `role_assigned`                 | A custom role remains assigned to a membership |
| 503  | `authentication_unavailable`    | Kratos or identity persistence is unavailable  |
| 503  | `tenant_context_unavailable`    | Tenant membership context is unavailable       |
| 503  | `authorization_unavailable`     | Authorization persistence is unavailable       |

## Integration events

State changes commit their audit and outbox records atomically. Events use the
internal v1 envelope and are never public webhook events by default.

| Type                              | Aggregate    | Data                                                    |
| --------------------------------- | ------------ | ------------------------------------------------------- |
| `identity.user_linked`            | `user`       | `user_id`                                               |
| `identity.platform_roles_changed` | `user`       | `user_id`, sorted `role_ids`                            |
| `tenant.created`                  | `tenant`     | `tenant_id`, `name`, `created_by_user_id`               |
| `membership.status_changed`       | `membership` | `membership_id`, `user_id`, `previous_status`, `status` |
| `membership.roles_changed`        | `membership` | `membership_id`, `user_id`, sorted `role_ids`           |
| `role.changed`                    | `role`       | `role_id`, `name`, `change`, sorted `permission_names`  |

Identity events are platform-scoped and therefore have a null envelope
`tenant_id`. Tenant, membership, and tenant-role events carry the owning tenant
ID. Event data excludes email addresses, Kratos identity IDs, session data, and
free-form audit reasons.

## Explicit deferrals

Fine-grained team/resource roles, customer identity/KYC, enterprise SSO,
production identity migration, invitation delivery, API keys, and public
identity webhooks are outside Phase 02.
