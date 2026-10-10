#!/usr/bin/env bash
set -euo pipefail

readonly repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly compose_file="${repository_root}/deploy/compose.yaml"
readonly project_name="credar-authentication-test-$$"
readonly port_offset=$(( $$ % 1000 ))
readonly postgres_port=$((43000 + port_offset))
readonly kratos_public_port=$((44000 + port_offset))
readonly kratos_admin_port=$((45000 + port_offset))

export CREDAR_POSTGRES_HOST_PORT="${postgres_port}"
export CREDAR_KRATOS_PUBLIC_HOST_PORT="${kratos_public_port}"
export CREDAR_KRATOS_ADMIN_HOST_PORT="${kratos_admin_port}"
export CREDAR_KRATOS_SESSION_LIFESPAN="2s"
export CREDAR_DATABASE_URL="postgres://credar:credar_local@postgres:5432/credar?sslmode=disable"

compose() {
  docker compose --project-name "${project_name}" --file "${compose_file}" "$@"
}

cleanup() {
  readonly exit_status=$?
  if ((exit_status != 0)); then
    mkdir -p "${repository_root}/artifacts"
    compose ps --all >"${repository_root}/artifacts/authentication-status.log" 2>&1 || true
    compose logs --no-color postgres kratos-migrate kratos >"${repository_root}/artifacts/authentication-services.log" 2>&1 || true
    echo "Authentication diagnostics written to artifacts/." >&2
  fi
  compose down --volumes --remove-orphans >/dev/null 2>&1 || true
  exit "${exit_status}"
}
trap cleanup EXIT

compose config --quiet
"${repository_root}/scripts/pull-compose-images.sh" \
  "${project_name}" "${compose_file}" postgres migrate kratos-migrate kratos
compose up --detach --wait --wait-timeout 180 postgres kratos
compose run --rm migrate up

(
  cd "${repository_root}/backend"
  CGO_ENABLED=0 \
    CREDAR_TEST_DATABASE_URL="postgres://credar:credar_local@127.0.0.1:${postgres_port}/credar?sslmode=disable" \
    CREDAR_TEST_KRATOS_PUBLIC_URL="http://127.0.0.1:${kratos_public_port}" \
    CREDAR_TEST_KRATOS_ADMIN_URL="http://127.0.0.1:${kratos_admin_port}" \
    go test -tags=integration ./internal/identity/httpapi -run TestAuthenticationAgainstKratos
)

echo "Kratos session validation and application identity mapping passed."
