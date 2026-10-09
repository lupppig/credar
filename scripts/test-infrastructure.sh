#!/usr/bin/env bash
set -euo pipefail

readonly repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly compose_file="${repository_root}/deploy/compose.yaml"
readonly project_name="credar-infra-test-$$"
readonly port_offset=$((20000 + ($$ % 10000)))

export CREDAR_POSTGRES_HOST_PORT=$((port_offset + 1))
export CREDAR_NATS_HOST_PORT=$((port_offset + 2))
export CREDAR_NATS_MONITOR_HOST_PORT=$((port_offset + 3))
export CREDAR_REDIS_HOST_PORT=$((port_offset + 4))
export CREDAR_SEAWEEDFS_HOST_PORT=$((port_offset + 5))
export CREDAR_KRATOS_PUBLIC_HOST_PORT=$((port_offset + 6))
export CREDAR_KRATOS_ADMIN_HOST_PORT=$((port_offset + 7))

compose() {
  docker compose --project-name "${project_name}" --file "${compose_file}" "$@"
}

cleanup() {
  readonly exit_status=$?
  if ((exit_status != 0)); then
    mkdir -p "${repository_root}/artifacts"
    compose ps --all >"${repository_root}/artifacts/infrastructure-status.log" 2>&1 || true
    compose logs --no-color >"${repository_root}/artifacts/infrastructure-services.log" 2>&1 || true
    echo "Infrastructure diagnostics written to artifacts/." >&2
  fi
  compose down --volumes --remove-orphans >/dev/null 2>&1 || true
  exit "${exit_status}"
}
trap cleanup EXIT

compose config --quiet
compose down --volumes --remove-orphans >/dev/null 2>&1 || true
compose up --detach --wait --wait-timeout 180 postgres nats redis seaweedfs kratos

postgres_database="$(compose exec -T postgres psql --username credar --dbname credar --tuples-only --no-align --command 'SELECT current_database()')"
[[ "${postgres_database}" == "credar" ]] || {
  echo "PostgreSQL did not answer from the expected database." >&2
  exit 1
}

kratos_database="$(compose exec -T postgres psql --username credar --dbname credar --tuples-only --no-align --command "SELECT datname FROM pg_database WHERE datname = 'kratos'")"
[[ "${kratos_database}" == "kratos" ]] || {
  echo "The Kratos database was not initialized." >&2
  exit 1
}

[[ "$(compose exec -T redis redis-cli ping)" == "PONG" ]] || {
  echo "Redis did not answer PING." >&2
  exit 1
}

curl --fail --silent --show-error "http://127.0.0.1:${CREDAR_NATS_MONITOR_HOST_PORT}/healthz?js-enabled-only=true" >/dev/null
curl --silent --show-error "http://127.0.0.1:${CREDAR_SEAWEEDFS_HOST_PORT}/" >/dev/null
curl --fail --silent --show-error "http://127.0.0.1:${CREDAR_KRATOS_ADMIN_HOST_PORT}/health/ready" >/dev/null

echo "PostgreSQL, NATS JetStream, Redis, SeaweedFS, and Kratos are healthy from empty volumes."
