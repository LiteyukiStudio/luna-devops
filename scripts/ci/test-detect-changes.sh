#!/usr/bin/env bash

set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
readonly DETECTOR="${ROOT_DIR}/scripts/ci/detect-changes.sh"

run_case() {
  local paths="$1"
  local output_file="$2"
  local paths_file
  paths_file="$(mktemp)"
  printf '%s\n' "${paths}" > "${paths_file}"
  GITHUB_OUTPUT="${output_file}" bash "${DETECTOR}" --files-from "${paths_file}" >/dev/null
  rm -f "${paths_file}"
}

assert_output() {
  local output_file="$1"
  local expected="$2"
  if ! grep -Fqx "${expected}" "${output_file}"; then
    printf 'missing output %q in:\n' "${expected}" >&2
    cat "${output_file}" >&2
    exit 1
  fi
}

output="$(mktemp)"
trap 'rm -f "${output}"' EXIT

run_case 'internal/api/router.go' "${output}"
assert_output "${output}" 'go=true'
assert_output "${output}" 'web=false'
assert_output "${output}" 'agent=false'
assert_output "${output}" 'docs=false'
assert_output "${output}" 'helm=false'
assert_output "${output}" 'container=true'
assert_output "${output}" 'container_images=["api"]'

: > "${output}"
run_case 'web/src/main.tsx' "${output}"
assert_output "${output}" 'go=false'
assert_output "${output}" 'web=true'
assert_output "${output}" 'agent=false'
assert_output "${output}" 'docs=false'
assert_output "${output}" 'container_images=["api"]'

: > "${output}"
run_case 'internal/worker/runner.go' "${output}"
assert_output "${output}" 'go=true'
assert_output "${output}" 'web=false'
assert_output "${output}" 'container_images=["worker"]'

: > "${output}"
run_case 'cmd/gateway-traffic-probe/main.go' "${output}"
assert_output "${output}" 'go=true'
assert_output "${output}" 'agent=false'
assert_output "${output}" 'container_images=["gateway-traffic-probe"]'

: > "${output}"
run_case 'luna-agent/src/index.ts' "${output}"
assert_output "${output}" 'go=false'
assert_output "${output}" 'web=false'
assert_output "${output}" 'agent=true'
assert_output "${output}" 'docs=false'
assert_output "${output}" 'container_images=["agent"]'

: > "${output}"
run_case 'docs/docs/zh/index.md' "${output}"
assert_output "${output}" 'docs=true'
assert_output "${output}" 'go=false'
assert_output "${output}" 'container=false'
assert_output "${output}" 'container_images=[]'

: > "${output}"
run_case '.github/workflows/build-publish.yml' "${output}"
assert_output "${output}" 'go=true'
assert_output "${output}" 'web=true'
assert_output "${output}" 'agent=true'
assert_output "${output}" 'docs=true'
assert_output "${output}" 'helm=true'
assert_output "${output}" 'container=true'
assert_output "${output}" 'container_images=["api","worker","agent","gateway-traffic-probe"]'

: > "${output}"
tag_paths="$(mktemp)"
printf '%s\n' 'docs/docs/zh/index.md' > "${tag_paths}"
GITHUB_EVENT_NAME=push GITHUB_REF=refs/tags/v1.2.3 GITHUB_OUTPUT="${output}" \
  bash "${DETECTOR}" --files-from "${tag_paths}" >/dev/null
rm -f "${tag_paths}"
assert_output "${output}" 'go=true'
assert_output "${output}" 'web=true'
assert_output "${output}" 'agent=true'
assert_output "${output}" 'docs=true'
assert_output "${output}" 'helm=true'
assert_output "${output}" 'container_images=["api","worker","agent","gateway-traffic-probe"]'

printf '%s\n' 'CI change detection tests passed.'
