#!/usr/bin/env bash
set -euo pipefail

readonly repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly smoke_url="http://127.0.0.1:13000"
readonly log_path="$(mktemp /tmp/credar-frontend-smoke.XXXXXX.log)"
readonly not_found_path="$(mktemp /tmp/credar-frontend-not-found.XXXXXX.html)"
readonly response_headers_path="$(mktemp /tmp/credar-frontend-headers.XXXXXX.txt)"

if [[ "${CREDAR_SMOKE_SKIP_BUILD:-false}" != "true" ]]; then
  (
    cd "${repository_root}"
    pnpm --dir frontend build
  )
fi

(
  cd "${repository_root}"
  pnpm --dir frontend start --hostname 127.0.0.1 --port 13000
) >"${log_path}" 2>&1 &
server_pid=$!
trap 'kill "${server_pid}" >/dev/null 2>&1 || true; wait "${server_pid}" >/dev/null 2>&1 || true; rm -f "${log_path}" "${not_found_path}" "${response_headers_path}"' EXIT

for _ in {1..100}; do
  if response="$(curl --dump-header "${response_headers_path}" --fail --silent --show-error "${smoke_url}" 2>/dev/null)"; then
    if [[ "${response}" == *"Launch credit."* ]]; then
      if ! grep -qi '^content-security-policy:.*frame-ancestors' "${response_headers_path}" ||
        ! grep -qi '^x-content-type-options: *nosniff' "${response_headers_path}" ||
        ! grep -qi '^x-frame-options: *DENY' "${response_headers_path}"; then
        echo "Frontend security headers were missing or incomplete." >&2
        exit 1
      fi
      not_found_status="$(curl --silent --show-error --output "${not_found_path}" --write-out '%{http_code}' "${smoke_url}/missing-route")"
      if [[ "${not_found_status}" != "404" ]] || ! grep -q "This console route does not exist." "${not_found_path}"; then
        echo "Frontend not-found state did not return its intentional 404 response." >&2
        exit 1
      fi
      echo "Frontend process smoke test passed."
      exit 0
    fi
    echo "Frontend shell did not contain its expected heading." >&2
    exit 1
  fi
  sleep 0.1
done

echo "Frontend did not become ready at ${smoke_url}." >&2
cat "${log_path}" >&2
exit 1
