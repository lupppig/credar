#!/usr/bin/env bash
set -euo pipefail

readonly repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly compose_file="${repository_root}/deploy/compose.yaml"
readonly project_name="credar-adapters-test-$$"
readonly port_offset=$((50000 + ($$ % 10000)))
readonly redis_port=$((port_offset + 1))
readonly seaweedfs_port=$((port_offset + 2))

export CREDAR_REDIS_HOST_PORT="${redis_port}"
export CREDAR_SEAWEEDFS_HOST_PORT="${seaweedfs_port}"

compose() {
  docker compose --project-name "${project_name}" --file "${compose_file}" "$@"
}

cleanup() {
  readonly exit_status=$?
  if ((exit_status != 0)); then
    mkdir -p "${repository_root}/artifacts"
    compose ps --all >"${repository_root}/artifacts/adapters-status.log" 2>&1 || true
    compose logs --no-color >"${repository_root}/artifacts/adapters-services.log" 2>&1 || true
    echo "Adapter infrastructure diagnostics written to artifacts/." >&2
  fi
  compose down --volumes --remove-orphans >/dev/null 2>&1 || true
  exit "${exit_status}"
}
trap cleanup EXIT

compose config --quiet
"${repository_root}/scripts/pull-compose-images.sh" \
  "${project_name}" "${compose_file}" redis seaweedfs
compose up --detach --wait --wait-timeout 120 redis seaweedfs

(
  cd "${repository_root}/backend"
  CGO_ENABLED=0 \
    CREDAR_TEST_REDIS_URL="redis://127.0.0.1:${redis_port}/0" \
    CREDAR_TEST_REDIS_KEY_PREFIX="credar_test_$$" \
    CREDAR_TEST_OBJECT_STORAGE_ENDPOINT="http://127.0.0.1:${seaweedfs_port}" \
    CREDAR_TEST_OBJECT_STORAGE_BUCKET="credar-test-$$" \
    CREDAR_TEST_OBJECT_STORAGE_ACCESS_KEY="local-access-key" \
    CREDAR_TEST_OBJECT_STORAGE_SECRET_KEY="local-secret-key" \
    go test -tags=integration \
      ./internal/platform/redisclient \
      ./internal/platform/objectstorage
)

echo "Redis isolation and opaque SeaweedFS object round trip passed."
