#!/usr/bin/env bash

set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly HELM_CHART_DIR="${ROOT_DIR}/charts/luna-devops"
readonly GOVULNCHECK_VERSION="v1.6.0"

section() {
  printf '\n==> %s\n' "$1"
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'required command not found: %s\n' "$1" >&2
    exit 1
  fi
}

cd "${ROOT_DIR}"

section "Checking release prerequisites"
require_command git

worktree_status="$(git status --porcelain=v1 --untracked-files=all)"
if [[ -n "${worktree_status}" ]]; then
  printf 'release checks require a clean Git worktree:\n%s\n' "${worktree_status}" >&2
  exit 1
fi

for command_name in go pnpm helm; do
  require_command "${command_name}"
done

if [[ -z "${AUTH_TEST_DATABASE_URL:-}" ]]; then
  printf 'AUTH_TEST_DATABASE_URL is required for PostgreSQL integration and migration tests\n' >&2
  exit 1
fi

if [[ ! -f "${ROOT_DIR}/.go-version" ]]; then
  printf '.go-version is missing\n' >&2
  exit 1
fi

if [[ ! -f "${HELM_CHART_DIR}/Chart.yaml" ]]; then
  printf 'Helm chart is missing: %s\n' "${HELM_CHART_DIR}/Chart.yaml" >&2
  exit 1
fi

expected_go_version="$(tr -d '[:space:]' < "${ROOT_DIR}/.go-version")"
module_go_version="$(awk '$1 == "go" { print $2; exit }' "${ROOT_DIR}/go.mod")"
installed_go_version="$(go env GOVERSION)"
module_go_series="$(printf '%s' "${module_go_version}" | cut -d. -f1,2)"
expected_go_series="$(printf '%s' "${expected_go_version}" | cut -d. -f1,2)"
if [[ "${module_go_series}" != "${expected_go_series}" ]]; then
  printf 'Go release series mismatch: go.mod=%s .go-version=%s\n' "${module_go_version}" "${expected_go_version}" >&2
  exit 1
fi
if [[ "${installed_go_version}" != "go${expected_go_version}" ]]; then
  printf 'Go toolchain mismatch: installed=%s expected=go%s\n' "${installed_go_version}" "${expected_go_version}" >&2
  exit 1
fi

section "Checking Go formatting"
unformatted_files=""
while IFS= read -r go_file; do
  if [[ -n "$(gofmt -l "${go_file}")" ]]; then
    unformatted_files+="${go_file}"$'\n'
  fi
done < <(git ls-files -- '*.go')
if [[ -n "${unformatted_files}" ]]; then
  printf 'gofmt is required for:\n%s' "${unformatted_files}" >&2
  exit 1
fi

section "Running Go tests"
AUTH_TEST_DATABASE_URL="" go test ./...

section "Running PostgreSQL integration and migration tests without cache"
go test -count=1 ./internal/api ./internal/database

section "Running Go vet"
go vet ./...

section "Running race tests for critical packages"
AUTH_TEST_DATABASE_URL="" go test -race ./internal/api ./internal/worker ./internal/provider/kubernetes ./internal/secret

section "Installing locked frontend dependencies"
pnpm --dir web install --frozen-lockfile
pnpm --dir docs install --frozen-lockfile

section "Linting and building the frontend"
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web build

section "Building the documentation site"
pnpm --dir docs build

section "Auditing pnpm dependencies"
pnpm --dir web audit --audit-level=high
pnpm --dir docs audit --audit-level=high

section "Scanning Go dependencies and reachable code"
go run "golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}" ./...

section "Linting and rendering the Helm chart"
helm lint "${HELM_CHART_DIR}"
rendered_chart="$(mktemp)"
trap 'rm -f "${rendered_chart}"' EXIT
helm template luna-devops "${HELM_CHART_DIR}" > "${rendered_chart}"
if [[ ! -s "${rendered_chart}" ]]; then
  printf 'Helm rendered an empty manifest\n' >&2
  exit 1
fi

section "Release checks passed"
