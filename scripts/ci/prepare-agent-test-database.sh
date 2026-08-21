#!/usr/bin/env bash

set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

if [[ -z "${AGENT_TEST_DATABASE_URL:-}" ]]; then
  printf '%s\n' 'AGENT_TEST_DATABASE_URL is required' >&2
  exit 1
fi

psql "${AGENT_TEST_DATABASE_URL}" \
  --set ON_ERROR_STOP=1 \
  --file "${ROOT_DIR}/luna-agent/sql/001_ai_schema.sql" \
  >/dev/null
