# Shared contract conventions

These conventions prevent modules and clients from inventing incompatible
transport behavior. Domain invariants remain in domain code; schemas describe
what crosses a process or persistence boundary.

## HTTP API

The source description is `contracts/openapi/platform.yaml` using OpenAPI
3.1.1. Application endpoints live under `/v1`. Health and operational probes
are deliberately unversioned because they are runtime contracts rather than
product resources.

Within an API major version, changes are backward compatible:

- optional request fields and response fields may be added;
- enum values may be added only when the schema and client guidance mark the
  enum as open;
- required fields, meanings, validation, status codes, and field types are not
  changed incompatibly;
- fields are deprecated before removal, and removal requires a new API major
  version.

JSON is the default representation. Errors use `application/problem+json` and
the common `Problem` schema. `code` is the stable programmatic discriminator;
human-readable titles and details are not API control flow.

## Request and correlation identifiers

Every response contains `X-Request-ID`, identifying one HTTP attempt. A valid
client-supplied UUIDv7 may be retained; otherwise the server generates one.

`X-Correlation-ID` connects a user intent across requests, transactions,
events, workers, and webhooks. A valid supplied UUIDv7 is propagated; otherwise
the boundary generates one. A retry receives a new request ID but can retain
the same correlation ID.

Protected tenant operations require an explicit `X-Tenant-ID`. Authentication,
membership, permission, and resource ownership are still verified server-side;
the header is context, not authorization.

The Phase 02 identity, membership lifecycle, built-in roles, permission
catalogue, errors, audit actions, and integration events are defined in
[`identity-authorization.md`](identity-authorization.md).

## Pagination

Collections use opaque cursor pagination with `page[limit]` and `page[after]`.
The default limit is 50 and the maximum is 200 unless an endpoint documents a
smaller bound. Queries use a deterministic order with an immutable unique ID as
the final tie-breaker. Responses return `page.next_cursor` and `page.has_more`.
Clients must not parse cursors.

## Idempotency

Mutation endpoints that create a financial or otherwise non-repeatable effect
declare the `Idempotency-Key` header in OpenAPI. Keys are opaque ASCII values of
8–255 characters.

An idempotency identity is scoped to tenant, authenticated principal, operation,
and key. The server stores a canonical request fingerprint and outcome:

- the same identity and fingerprint returns the original status and body;
- the same identity with a different fingerprint returns HTTP 409 and
  `idempotency_key_reused`;
- concurrent duplicates serialize behind one owner or receive an explicit
  retryable in-progress response; they never execute the effect twice;
- the endpoint documents retention, with a platform minimum of 24 hours.

Database constraints and transactional records enforce idempotency. Process
memory is not an idempotency store.

## Events

Domain events are internal domain facts. Integration events are versioned
messages emitted after commit through the outbox. Public webhook events are a
separate, deliberately selected catalog. Mapping between these layers is
explicit; an internal event is never public merely because it exists.

Integration envelopes follow
`contracts/events/internal/envelope-v1.schema.json`. Public webhook envelopes
follow `contracts/events/public/envelope-v1.schema.json`.

- Event IDs are UUIDv7 and remain stable across broker redelivery.
- `type` uses lower snake-separated domain names such as `loan.disbursed`.
- `version` is a positive integer scoped to the event type.
- Published schemas are immutable. A breaking payload change increments the
  version and creates a new schema.
- Consumers are idempotent by event ID and do not assume global ordering.
- `correlation_id` traces the originating intent; `causation_id` identifies the
  request or event that directly caused this event.

Internal event subjects use `credar.events.<type>.v<version>`. Each consumer
owns a stable, descriptive durable name ending in its contract version, such as
`credar-infrastructure-proof-v1`; changing consumer behavior incompatibly
requires a new durable version. Delivery is at least once. Producers preserve
the envelope event ID across retries, and consumers commit an event-ID receipt
in the same database transaction as their side effects before acknowledging
the broker message.

Poison messages exhaust a bounded retry policy before a diagnostic record is
published under `credar.dead.<original-subject>`. Dead-letter diagnostics may
identify the consumer, event, subject, delivery count, failure reason, and a
payload hash, but do not copy the original payload because it may contain
sensitive data.

## Permissions

Permission names use lowercase `<resource>:<action>`, for example
`applications:approve` or `webhooks:manage`. Names describe a capability, not a
screen or implementation. Platform-only permissions use a `platform.` resource
prefix, such as `platform.tenants:create`, and cannot be granted by tenant
custom roles.

Authorization combines authenticated identity, active membership, permission,
and resource scope. A permission never implies cross-tenant access.

## Audit metadata

Consequential authorization, identity, credit, risk, and financial changes
carry the metadata defined by `contracts/audit/metadata-v1.schema.json`.
Actor, tenant, request, correlation, source, and rationale are recorded when
applicable. Sensitive request bodies, credentials, secrets, and arbitrary
provider payloads do not belong in audit metadata.
