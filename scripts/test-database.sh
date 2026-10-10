#!/usr/bin/env bash
set -euo pipefail

readonly repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly compose_file="${repository_root}/deploy/compose.yaml"
readonly project_name="credar-database-test-$$"
readonly postgres_port=$((30000 + ($$ % 10000)))

export CREDAR_POSTGRES_HOST_PORT="${postgres_port}"
export CREDAR_DATABASE_URL="postgres://credar:credar_local@postgres:5432/credar?sslmode=disable"

compose() {
  docker compose --project-name "${project_name}" --file "${compose_file}" "$@"
}

cleanup() {
  readonly exit_status=$?
  if ((exit_status != 0)); then
    mkdir -p "${repository_root}/artifacts"
    compose ps --all >"${repository_root}/artifacts/database-status.log" 2>&1 || true
    compose logs --no-color >"${repository_root}/artifacts/database-services.log" 2>&1 || true
    echo "Database diagnostics written to artifacts/." >&2
  fi
  compose down --volumes --remove-orphans >/dev/null 2>&1 || true
  exit "${exit_status}"
}
trap cleanup EXIT

compose config --quiet
"${repository_root}/scripts/pull-compose-images.sh" \
  "${project_name}" "${compose_file}" postgres migrate
compose up --detach --wait --wait-timeout 120 postgres

compose run --rm migrate up
schema_exists="$(compose exec -T postgres psql --username credar --dbname credar --tuples-only --no-align --command "SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'platform')")"
[[ "${schema_exists}" == "t" ]] || {
  echo "The platform schema was not created." >&2
  exit 1
}

compose run --rm migrate down -all
schema_exists="$(compose exec -T postgres psql --username credar --dbname credar --tuples-only --no-align --command "SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'platform')")"
[[ "${schema_exists}" == "f" ]] || {
  echo "The platform schema was not removed by rollback." >&2
  exit 1
}

compose run --rm migrate up
(
  cd "${repository_root}/backend"
  export CGO_ENABLED=0
  export CREDAR_TEST_DATABASE_URL="postgres://credar:credar_local@127.0.0.1:${postgres_port}/credar?sslmode=disable"
  go test -tags=integration ./internal/platform/database
  go test -tags=integration ./internal/identity/postgres
  go test -tags=integration ./internal/tenancy/postgres
)

echo "Database migrations, pooling, readiness, commit, and rollback behavior passed."
