#!/usr/bin/env bash
set -euo pipefail

if (($# < 3)); then
  echo "Usage: $0 <project-name> <compose-file> <service> [service ...]" >&2
  exit 2
fi

readonly project_name="$1"
readonly compose_file="$2"
shift 2

readonly max_attempts=3

for ((attempt = 1; attempt <= max_attempts; attempt++)); do
  if docker compose \
    --project-name "${project_name}" \
    --file "${compose_file}" \
    pull "$@"; then
    exit 0
  fi

  if ((attempt == max_attempts)); then
    echo "Unable to pull Compose images after ${max_attempts} attempts." >&2
    exit 1
  fi

  delay=$((attempt * 15))
  echo "Compose image pull failed; retrying in ${delay} seconds." >&2
  sleep "${delay}"
done
