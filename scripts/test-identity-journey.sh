#!/usr/bin/env bash
set -euo pipefail

readonly repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

pnpm --dir "${repository_root}/frontend" test
pnpm --dir "${repository_root}/frontend" build
make --directory "${repository_root}" authentication-test

echo "Identity console states and the real Kratos tenant authorization journey passed."
