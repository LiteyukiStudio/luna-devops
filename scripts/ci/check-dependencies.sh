#!/usr/bin/env bash

set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
readonly GOVULNCHECK_VERSION="v1.6.0"
readonly -a PNPM_AUDIT_NETWORK_ARGS=(
  --fetch-retries=5
  --fetch-retry-factor=2
  --fetch-retry-mintimeout=5000
  --fetch-retry-maxtimeout=20000
  --fetch-timeout=30000
)

cd "${ROOT_DIR}"

node "${ROOT_DIR}/scripts/ci/test-check-dependencies.mjs"

# GHSA-qwww-vcr4-c8h2 only affects React Router's unstable RSC APIs, which
# neither the Vite SPA nor Rspress documentation site enables. The narrow
# exemptions are declared in each project's pnpm-workspace.yaml.
pnpm --dir web audit --prod --audit-level=high "${PNPM_AUDIT_NETWORK_ARGS[@]}"
pnpm --dir docs audit --prod --audit-level=high "${PNPM_AUDIT_NETWORK_ARGS[@]}"
pnpm --dir luna-agent audit --prod --audit-level=high "${PNPM_AUDIT_NETWORK_ARGS[@]}"
go run "golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}" ./...
