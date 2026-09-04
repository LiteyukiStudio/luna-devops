#!/usr/bin/env bash

set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly HELM_CHART_DIR="${ROOT_DIR}/charts/luna-devops"

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

for command_name in go pnpm node helm psql; do
  require_command "${command_name}"
done

if [[ -z "${AUTH_TEST_DATABASE_URL:-}" ]]; then
  printf 'AUTH_TEST_DATABASE_URL is required for PostgreSQL integration and migration tests\n' >&2
  exit 1
fi
if [[ -z "${AGENT_TEST_DATABASE_URL:-}" ]]; then
  printf 'AGENT_TEST_DATABASE_URL is required for Agent PostgreSQL integration tests\n' >&2
  exit 1
fi

auth_test_database="$(psql "${AUTH_TEST_DATABASE_URL}" -Atqc 'select current_database()')"
agent_test_database="$(psql "${AGENT_TEST_DATABASE_URL}" -Atqc 'select current_database()')"
if [[ -z "${auth_test_database}" || -z "${agent_test_database}" ]]; then
  printf 'unable to resolve the PostgreSQL test database names\n' >&2
  exit 1
fi
if [[ "${auth_test_database}" == "${agent_test_database}" ]]; then
  printf 'AUTH_TEST_DATABASE_URL and AGENT_TEST_DATABASE_URL must use isolated databases\n' >&2
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

section "Checking Go formatting, tests, and vet"
bash "${ROOT_DIR}/scripts/ci/check-go.sh"

section "Running PostgreSQL integration and migration tests without cache"
bash "${ROOT_DIR}/scripts/ci/check-go-postgres.sh"

section "Running race tests for critical packages"
bash "${ROOT_DIR}/scripts/ci/check-go-race.sh"

section "Installing locked frontend dependencies"
pnpm --dir luna-agent install --frozen-lockfile
pnpm --dir web install --frozen-lockfile
pnpm --dir docs install --frozen-lockfile

section "Checking the AI Agent"
bash "${ROOT_DIR}/scripts/ci/prepare-agent-test-database.sh"
bash "${ROOT_DIR}/scripts/ci/check-agent.sh"

section "Linting and building the frontend"
bash "${ROOT_DIR}/scripts/ci/check-web.sh"

section "Building the documentation site"
bash "${ROOT_DIR}/scripts/ci/check-docs.sh"

section "Auditing dependency vulnerabilities"
bash "${ROOT_DIR}/scripts/ci/check-dependencies.sh"

section "Linting, rendering, and testing the Helm chart"
bash "${ROOT_DIR}/scripts/ci/check-helm.sh"

section "Release checks passed"
