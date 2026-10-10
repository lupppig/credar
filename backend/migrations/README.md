# Database migrations

Phase 01 introduces the database runtime and first migration. Migration files
are ordered, paired plain SQL files:

```text
000001_descriptive_name.up.sql
000001_descriptive_name.down.sql
```

Numbers are six digits and never reused. Names use lowercase snake case. Every
`up` file has a matching `down` file with the same number and name. A down
migration documents irreversible behavior instead of pretending data can be
restored.

Migrations run explicitly through the pinned `golang-migrate` container. The
API never applies migrations during startup. Run `make migrate-up`,
`make migrate-down`, or `make migrate-version` for a configured local stack,
and run `make database-test` to prove empty-database forward, rollback, and
re-apply behavior.

## Schema conventions

- PostgreSQL `uuid` columns hold application-generated UUIDv7 values. Random or
  sequential database IDs are not introduced independently by repositories.
- Persisted instants use `timestamptz`, are interpreted as UTC, and use names
  ending in `_at`. Business dates use `date` only when wall-clock time has no
  meaning.
- Tenant-owned tables use a non-null `tenant_id`. Their uniqueness constraints
  and lookup indexes begin with `tenant_id` unless the documented access path
  proves a different leading column is required.
- Foreign keys and check constraints are explicit. Indexes are added for real
  query and locking paths, not pre-emptively for every column.
- Monetary amounts use integer minor units plus an ISO currency code; binary
  floating point is prohibited.

The initial migration creates the `platform` schema reserved for runtime
infrastructure. The second migration adds the transactional outbox and durable
consumer receipts within that schema. Migration `000004` introduces the
`identity` capability schema with application-user mappings and immutable audit
events. Migration `000005` adds the `tenancy` schema, tenant-scoped membership
constraints, and durable platform idempotency records. Later domain migrations
remain owned by their delivery tasks. Migration `000006` seeds the closed
permission catalogue and immutable system roles, adds platform and membership
role assignments, and enforces role/permission/tenant scope at the database
boundary.
