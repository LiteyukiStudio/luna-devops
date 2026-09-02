#!/usr/bin/env bash

set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

if [[ -z "${AUTH_TEST_DATABASE_URL:-}" ]]; then
  printf '%s\n' 'AUTH_TEST_DATABASE_URL is required' >&2
  exit 1
fi

# These packages own the release-blocking authentication/API integration and
# fresh/upgrade migration paths. Keep package execution serial because the
# migration suites perform DDL against the same disposable admin database.
go test -p 1 -count=1 ./internal/api/... ./internal/database
