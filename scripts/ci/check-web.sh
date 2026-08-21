#!/usr/bin/env bash

set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

pnpm --dir web test
pnpm --dir web lint
pnpm --dir web check:singletons
pnpm --dir web build
