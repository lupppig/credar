#!/usr/bin/env bash
set -euo pipefail

readonly expected_go="go1.27.2"
readonly expected_node="v24.21.0"
readonly expected_pnpm="12.5.1"
failed=0

require_command() {
  local command_name="$1"
  local install_hint="$2"

  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "Missing ${command_name}. ${install_hint}" >&2
    failed=1
  fi
}

require_command go "Install the version declared in .tool-versions."
require_command node "Install the version declared in .nvmrc or .tool-versions."
require_command pnpm "Install the version declared in .tool-versions or activate the root packageManager version."
require_command docker "Install Docker Engine with the Compose plugin."

if command -v go >/dev/null 2>&1; then
  actual_go="$(go version | awk '{print $3}')"
  if [[ "${actual_go}" != "${expected_go}" ]]; then
    echo "Go version mismatch: expected ${expected_go}, found ${actual_go}." >&2
    failed=1
  fi
fi

if command -v node >/dev/null 2>&1; then
  actual_node="$(node --version)"
  if [[ "${actual_node}" != "${expected_node}" ]]; then
    echo "Node.js version mismatch: expected ${expected_node}, found ${actual_node}." >&2
    failed=1
  fi
fi

if command -v pnpm >/dev/null 2>&1; then
	actual_pnpm="$(pnpm --version 2>/dev/null || true)"
	if [[ "${actual_pnpm}" != "${expected_pnpm}" ]]; then
		echo "pnpm version mismatch: expected ${expected_pnpm}, found ${actual_pnpm:-unavailable}." >&2
		echo "Use .tool-versions or the root packageManager pin to activate it." >&2
		failed=1
	fi
fi

if command -v docker >/dev/null 2>&1 && ! docker compose version >/dev/null 2>&1; then
  echo "Docker Compose plugin is unavailable. Install it before running local services." >&2
  failed=1
fi

if [[ "${failed}" -ne 0 ]]; then
  exit 1
fi

echo "Required repository toolchains are available."
