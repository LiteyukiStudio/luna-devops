#!/usr/bin/env bash

set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

exec node "${ROOT_DIR}/scripts/generate-changelog.mjs" "$@"
