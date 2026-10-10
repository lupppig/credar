#!/usr/bin/env bash
set -euo pipefail

readonly repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly compose_file="${repository_root}/deploy/compose.yaml"
readonly project_name="credar-full-stack-test-$$"
readonly event_id="019b1234-0000-7000-8000-000000000001"
readonly aggregate_id="019b1234-0000-7000-8000-000000000002"
readonly correlation_id="019b1234-0000-7000-8000-000000000003"
readonly causation_id="019b1234-0000-7000-8000-000000000004"
readonly request_id="019b1234-0000-7000-8000-000000000005"
readonly backend_headers="$(mktemp /tmp/credar-full-stack-headers.XXXXXX)"
readonly backend_logs="$(mktemp /tmp/credar-full-stack-logs.XXXXXX)"

export CREDAR_POSTGRES_HOST_PORT=0
export CREDAR_NATS_HOST_PORT=0
export CREDAR_NATS_MONITOR_HOST_PORT=0
export CREDAR_REDIS_HOST_PORT=0
export CREDAR_SEAWEEDFS_HOST_PORT=0
export CREDAR_KRATOS_PUBLIC_HOST_PORT=0
export CREDAR_KRATOS_ADMIN_HOST_PORT=0
export CREDAR_BACKEND_HOST_PORT=0
export CREDAR_FRONTEND_HOST_PORT=0
export CREDAR_DATABASE_URL="postgres://credar:credar_local@postgres:5432/credar?sslmode=disable"
export CREDAR_NATS_URL="nats://nats:4222"
export CREDAR_REDIS_URL="redis://redis:6379/0"
export CREDAR_REDIS_KEY_PREFIX="credar_full_stack_$$"
export CREDAR_OBJECT_STORAGE_ENDPOINT="http://seaweedfs:8333"
export CREDAR_OBJECT_STORAGE_BUCKET="credar-full-stack-$$"
export CREDAR_OBJECT_STORAGE_ACCESS_KEY="local-access-key"
export CREDAR_OBJECT_STORAGE_SECRET_KEY="local-secret-key"

compose() {
  docker compose --project-name "${project_name}" --file "${compose_file}" "$@"
}

cleanup() {
  local exit_status=$?
  if ((exit_status != 0)); then
    mkdir -p "${repository_root}/artifacts"
    compose ps --all >"${repository_root}/artifacts/full-stack-status.log" 2>&1 || true
    compose logs --no-color >"${repository_root}/artifacts/full-stack-services.log" 2>&1 || true
    echo "Full-stack diagnostics written to artifacts/." >&2
  fi
  compose down --volumes --remove-orphans >/dev/null 2>&1 || true
  rm -f "${backend_headers}" "${backend_logs}"
  exit "${exit_status}"
}
trap cleanup EXIT

compose config --quiet
compose down --volumes --remove-orphans >/dev/null 2>&1 || true
"${repository_root}/scripts/pull-compose-images.sh" \
  "${project_name}" "${compose_file}" \
  postgres migrate nats redis seaweedfs kratos-migrate kratos
compose up --detach --build --wait --wait-timeout 240 postgres nats redis seaweedfs kratos backend

readonly backend_url="http://$(compose port backend 8080)"
export NEXT_PUBLIC_CREDAR_API_BASE_URL="${backend_url}"
compose build frontend
compose up --detach --no-build --wait --wait-timeout 240 frontend
readonly frontend_url="http://$(compose port frontend 3000)"

health_response="$(curl --fail --silent --show-error "${backend_url}/healthz")"
[[ "${health_response}" == '{"status":"ok"}' ]] || {
  echo "Backend liveness returned an unexpected response: ${health_response}" >&2
  exit 1
}

curl \
  --dump-header "${backend_headers}" \
  --fail \
  --silent \
  --show-error \
  --header "X-Request-ID: ${request_id}" \
  --header "X-Correlation-ID: ${correlation_id}" \
  "${backend_url}/readyz" | grep -q '"status":"ok"'
grep -qi "^X-Request-ID: ${request_id}" "${backend_headers}"
grep -qi "^X-Correlation-ID: ${correlation_id}" "${backend_headers}"

frontend_response="$(curl --fail --silent --show-error "${frontend_url}/")"
[[ "${frontend_response}" == *"Operational foundations, ready for product work."* ]] || {
  echo "Frontend did not serve the built enterprise shell." >&2
  exit 1
}

compose exec -T postgres psql \
  --username credar \
  --dbname credar \
  --set ON_ERROR_STOP=1 \
  --command "
    BEGIN;
    INSERT INTO platform.outbox_events (
      id, tenant_id, event_type, event_version, aggregate_type, aggregate_id,
      subject, occurred_at, correlation_id, causation_id, payload
    ) VALUES (
      '${event_id}', NULL, 'infrastructure.probe', 1,
      'infrastructure_probe', '${aggregate_id}',
      'credar.events.infrastructure.probe.v1', now(),
      '${correlation_id}', '${causation_id}',
      jsonb_build_object(
        'id', '${event_id}',
        'type', 'infrastructure.probe',
        'version', 1,
        'occurred_at', now(),
        'tenant_id', NULL,
        'aggregate', jsonb_build_object(
          'type', 'infrastructure_probe',
          'id', '${aggregate_id}'
        ),
        'correlation_id', '${correlation_id}',
        'causation_id', '${causation_id}',
        'data', jsonb_build_object('probe', 'full_stack')
      )
    );
    COMMIT;
  " >/dev/null

proof_state=""
for _ in {1..60}; do
  proof_state="$(compose exec -T postgres psql \
    --username credar \
    --dbname credar \
    --tuples-only \
    --no-align \
    --command "
      SELECT event.status || ':' || event.attempt_count || ':' || (
        SELECT COUNT(*)
        FROM platform.event_consumptions AS consumption
        WHERE consumption.consumer_name = 'credar-infrastructure-proof-v1'
          AND consumption.event_id = event.id
      )
      FROM platform.outbox_events AS event
      WHERE event.id = '${event_id}';
    ")"
  if [[ "${proof_state}" == "published:1:1" ]]; then
    break
  fi
  sleep 0.5
done

[[ "${proof_state}" == "published:1:1" ]] || {
  echo "Infrastructure event proof did not complete exactly once; state=${proof_state:-missing}." >&2
  exit 1
}

logs_correlate=false
for _ in {1..30}; do
  compose logs --no-color backend >"${backend_logs}"
  if grep -F '"msg":"http request completed"' "${backend_logs}" |
    grep -F '"path":"/readyz"' |
    grep -F "\"correlation_id\":\"${correlation_id}\"" >/dev/null &&
    grep -F '"msg":"outbox event publication completed"' "${backend_logs}" |
      grep -F "\"event_id\":\"${event_id}\"" |
      grep -F "\"correlation_id\":\"${correlation_id}\"" >/dev/null &&
    grep -F '"msg":"event consumption completed"' "${backend_logs}" |
      grep -F "\"event_id\":\"${event_id}\"" |
      grep -F "\"correlation_id\":\"${correlation_id}\"" >/dev/null; then
    logs_correlate=true
    break
  fi
  sleep 0.2
done

[[ "${logs_correlate}" == "true" ]] || {
  echo "Structured logs did not correlate the readiness, relay, and consumer path." >&2
  exit 1
}

echo "Built frontend and backend artifacts are healthy; the committed event was published and consumed exactly once."
