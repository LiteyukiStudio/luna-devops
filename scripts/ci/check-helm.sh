#!/usr/bin/env bash

set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
readonly CHART_DIR="${ROOT_DIR}/charts/luna-devops"
cd "${ROOT_DIR}"

helm lint "${CHART_DIR}"
rendered_chart="$(mktemp)"
trap 'rm -f "${rendered_chart}"' EXIT
helm template luna-devops "${CHART_DIR}" > "${rendered_chart}"
if [[ ! -s "${rendered_chart}" ]]; then
  printf '%s\n' 'Helm rendered an empty manifest' >&2
  exit 1
fi
sh "${CHART_DIR}/tests/render_test.sh" "${CHART_DIR}"
