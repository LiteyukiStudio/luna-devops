#!/usr/bin/env bash

set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

if [[ -z "${AGENT_TEST_DATABASE_URL:-}" ]]; then
  printf '%s\n' 'AGENT_TEST_DATABASE_URL is required' >&2
  exit 1
fi

pnpm --dir luna-agent exec vitest run \
  tests/postgres-repository.test.ts \
  tests/postgres-model-budget.test.ts
