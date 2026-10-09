# P01-06 handoff — Next.js runtime shell

## Outcome

The frontend is now an intentional enterprise-console shell with accessible,
responsive application chrome and explicit loading, route-error, global-error,
and not-found states. Browser and server API origins are validated, and one
server-side backend client owns correlation propagation and problem-response
normalization.

## Decisions

- The shell describes only implemented infrastructure capabilities. Sign-in,
  tenant selection, permissions, and product navigation remain visible future
  extension points rather than non-functional controls.
- Public and internal backend origins are separate. Both accept only clean
  absolute HTTP(S) origins; only the `NEXT_PUBLIC_` value is embedded in the
  browser bundle.
- Backend calls require a canonical UUIDv7 correlation ID. The client creates
  one when absent, forwards an explicit valid one, and retains response request
  and correlation IDs on normalized errors.
- Error UI records only the framework digest and never renders raw exceptions.
  Loading and failure states use semantic live-region/alert behavior, visible
  actions, keyboard focus styles, and reduced-motion support.

## Contracts and migrations

- Added no HTTP endpoint, event schema, database migration, authentication
  assumption, tenant context, permission, or product route.
- Added validated handling for `NEXT_PUBLIC_CREDAR_API_BASE_URL` and
  `CREDAR_BACKEND_INTERNAL_URL`; their existing names and local defaults are
  unchanged.
- The backend client consumes the existing `X-Correlation-ID`, `X-Request-ID`,
  JSON, and `application/problem+json` conventions.

## Verification

- Frontend unit tests cover environment validation, UUIDv7 generation,
  correlation propagation, problem mapping, honest shell content, and global
  route states.
- Prettier, ESLint with zero warnings, TypeScript, and the production Next.js
  build pass with the pinned toolchain.
- The process smoke test verifies the rendered home shell and an intentional
  unknown-route `404` response.
- The non-root standalone frontend container builds and serves the same shell.

## Remaining risks and next task

No application workflow consumes the backend client yet; the full-stack harness
will prove built-artifact connectivity before identity and tenant work begins.
Authentication, authorization, tenant selection, product navigation, and
domain-specific empty/denied states remain deliberately deferred.

Next: **P01-07 — Full-stack integration harness**.
