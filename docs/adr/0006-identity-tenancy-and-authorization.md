# ADR 0006: Application-owned tenant authorization

- Status: Accepted
- Date: 2026-10-10

## Context

Kratos provides identity and session flows but does not model Credar tenants,
memberships, business permissions, or resource ownership. Phase 02 requires one
authorization boundary before protected product APIs are introduced.

## Decision

Kratos is authoritative only for authentication credentials, recovery,
verification, and session validity. Credar maps each Kratos identity to one
application user and owns tenants, memberships, roles, permissions, resource
scope, and audit history in PostgreSQL.

Tenant context is explicit per protected request through `X-Tenant-ID`. It is
not persisted as a global server-side selection and does not grant access. The
server evaluates authenticated user status, active membership, permission, and
tenant-scoped resource ownership for every protected use case.

Use immutable seeded platform and tenant system roles plus tenant-owned custom
roles. Custom roles may contain only permissions from the tenant catalogue;
platform permissions and system roles cannot be created or changed by tenant
administrators. Consequential authorization changes append audit and outbox
records in the same transaction as the state change.

Platform roles attach directly to application users; tenant roles attach to
memberships. Bootstrap the first platform administrator through an explicit
one-time operator command that refuses to run after a platform assignment
exists. Authentication alone never bootstraps privilege.

Cross-tenant resource lookup is indistinguishable from absence. Authorization
policy remains application/domain code; Gin middleware extracts verified
context but does not hide resource-specific ownership decisions.

## Alternatives considered

- Storing roles in Kratos traits would couple business authorization to an
  identity-provider document, weaken relational constraints, and make tenant
  isolation harder to audit.
- Persisting one active tenant in the user session would create stale or
  conflicting context across browser tabs and API clients.
- Encoding all authorization in generic HTTP middleware would obscure
  resource ownership and encourage broad fetches followed by in-memory checks.

## Consequences

Every protected use case receives explicit authenticated identity and tenant
context. Database queries and constraints carry tenant scope. Kratos remains
replaceable at the authentication adapter boundary, while membership and role
history stay authoritative in PostgreSQL. Clients must send `X-Tenant-ID` for
tenant endpoints and treat tenant selection as request context rather than a
durable entitlement.
