#!/usr/bin/env bash

set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
readonly GOVULNCHECK_VERSION="v1.6.0"

cd "${ROOT_DIR}"

# GHSA-qwww-vcr4-c8h2 only affects React Router's unstable RSC APIs, which
# neither the Vite SPA nor Rspress documentation site enables.
pnpm --dir web audit --prod --audit-level=high --ignore=GHSA-qwww-vcr4-c8h2
pnpm --dir docs audit --prod --audit-level=high --ignore=GHSA-qwww-vcr4-c8h2
pnpm --dir luna-agent audit --prod --audit-level=high
go run "golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}" ./...
