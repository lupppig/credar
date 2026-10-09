# P01-04 handoff — Outbox and JetStream foundation

## Outcome

Committed integration events now move from PostgreSQL to a managed JetStream
stream through a transactional outbox relay. Relay claims are leased, publishing
happens outside database transactions, failures retry with bounded exponential
backoff, and exhausted records become dead letters. A durable proof consumer
records event IDs transactionally and acknowledges only after commit.

## Decisions

- Application code inserts an envelope and its domain changes through the same
  caller-owned transaction; the outbox repository never commits independently.
- Relays claim work with `FOR UPDATE SKIP LOCKED` in short transactions. Stable
  worker IDs and expiring leases make abandoned work recoverable.
- The runtime owns one `CREDAR_EVENTS` stream for `credar.events.>` and
  `credar.dead.>` subjects. Event subjects encode type and schema version.
- JetStream delivery is at least once. Consumer receipts keyed by consumer and
  event ID make database side effects idempotent before acknowledgement.
- Dead-letter messages contain bounded diagnostics and a SHA-256 payload hash,
  not the original potentially sensitive event body.

## Contracts and migrations

- Added `000002_outbox_and_consumers.up.sql` and its reversible down migration.
  It creates `platform.outbox_events` and `platform.event_consumptions` with
  lifecycle constraints and indexes for claim and receipt paths.
- Added the validated internal event-envelope representation and the subject
  convention `credar.events.<type>.v<version>`.
- Added validated NATS and outbox relay configuration. Connection URLs are
  treated as secrets and redact themselves during formatting.
- No product-domain API, tenant table, or financial table was introduced.

## Verification

- Backend unit tests cover envelope validation, relay success/retry behavior,
  outbox preconditions, and NATS client construction.
- `make event-test` starts isolated PostgreSQL and NATS instances, migrates from
  an empty database, and proves commit-to-publish-to-consume behavior.
- The integration test also proves rollback exclusion, expired-lease recovery,
  and one transactional receipt when the same event is delivered twice.
- Race-enabled backend tests, `go vet`, contract validation, Compose validation,
  migration pairing, formatting, and `git diff --check` pass.

## Remaining risks and next task

The proof consumer intentionally handles only the infrastructure probe event;
domain consumers arrive with their owning modules. Stream limits and retry
settings are safe local defaults, not a production capacity plan. Redis and
SeaweedFS still lack application-owned adapters and readiness integration.

Next: **P01-05 — Infrastructure adapters**.
