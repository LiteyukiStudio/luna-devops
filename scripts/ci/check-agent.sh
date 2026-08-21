#!/usr/bin/env bash

set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

pnpm --dir luna-agent lint
pnpm --dir luna-agent typecheck
pnpm --dir luna-agent test
pnpm --dir luna-agent build
