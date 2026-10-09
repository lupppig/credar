#!/usr/bin/env bash
set -euo pipefail

readonly repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly smoke_address="127.0.0.1:18080"
readonly smoke_url="http://${smoke_address}/healthz"
readonly binary_path="${repository_root}/bin/credar-api"
readonly log_path="$(mktemp /tmp/credar-backend-smoke.XXXXXX.log)"

mkdir -p "${repository_root}/bin"
(
  cd "${repository_root}/backend"
  go build -trimpath -o "${binary_path}" ./cmd/api
)

CREDAR_HTTP_ADDRESS="${smoke_address}" "${binary_path}" >"${log_path}" 2>&1 &
server_pid=$!
trap 'kill "${server_pid}" >/dev/null 2>&1 || true; wait "${server_pid}" >/dev/null 2>&1 || true; rm -f "${log_path}"' EXIT

for _ in {1..30}; do
  if response="$(curl --fail --silent --show-error "${smoke_url}" 2>/dev/null)"; then
    if [[ "${response}" == '{"status":"ok"}' ]]; then
      echo "Backend process smoke test passed."
      exit 0
    fi
    echo "Unexpected backend health response: ${response}" >&2
    exit 1
  fi
  sleep 0.1
done

echo "Backend did not become healthy at ${smoke_url}." >&2
cat "${log_path}" >&2
exit 1
