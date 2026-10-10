# Architecture decision records

Architecture decision records (ADRs) capture decisions that constrain multiple
parts of the platform or would be expensive to reverse after domain work
begins.

## Status values

- **Proposed** — under review and not yet binding.
- **Accepted** — the repository must follow the decision.
- **Superseded** — replaced by a newer ADR that links back to it.
- **Rejected** — considered but intentionally not selected.

Changing an accepted decision requires a new ADR. The new record must explain
the compatibility and migration consequences and mark the old record as
superseded; accepted records are not silently rewritten.

## Index

| ADR                                                       | Decision                                        | Status   |
| --------------------------------------------------------- | ----------------------------------------------- | -------- |
| [0001](0001-repository-layout-and-toolchains.md)          | Repository layout and application toolchains    | Accepted |
| [0002](0002-api-persistence-and-schema-tooling.md)        | API, persistence, migration, and schema tooling | Accepted |
| [0003](0003-identifiers-money-and-time.md)                | Identifiers, money, and time representation     | Accepted |
| [0004](0004-github-actions-for-continuous-integration.md) | GitHub Actions for continuous integration       | Accepted |
| [0005](0005-gin-http-transport.md)                        | Gin for the HTTP transport                      | Accepted |
| [0006](0006-identity-tenancy-and-authorization.md)        | Application-owned tenant authorization          | Accepted |
