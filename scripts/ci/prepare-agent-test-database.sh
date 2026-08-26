#!/usr/bin/env bash

set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

if [[ -z "${AGENT_TEST_DATABASE_URL:-}" ]]; then
  printf '%s\n' 'AGENT_TEST_DATABASE_URL is required' >&2
  exit 1
fi

while IFS= read -r migration; do
  psql "${AGENT_TEST_DATABASE_URL}" \
    --set ON_ERROR_STOP=1 \
    --file "${migration}" \
    >/dev/null
done < <(find "${ROOT_DIR}/migrations" -maxdepth 1 -type f -name '*.up.sql' -print | sort)
