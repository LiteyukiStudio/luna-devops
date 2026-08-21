#!/usr/bin/env bash

set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

AUTH_TEST_DATABASE_URL="" AGENT_OBSERVABILITY_TEST_DATABASE_URL="" \
  go test -race ./internal/api ./internal/worker ./internal/provider/kubernetes ./internal/secret
