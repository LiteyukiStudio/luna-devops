#!/usr/bin/env bash

set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

unformatted_files=""
while IFS= read -r -d '' go_file; do
  [[ -f "${go_file}" ]] || continue
  if [[ -n "$(gofmt -l "${go_file}")" ]]; then
    unformatted_files+="${go_file}"$'\n'
  fi
done < <(git ls-files -z --cached --others --exclude-standard -- '*.go')
if [[ -n "${unformatted_files}" ]]; then
  printf 'gofmt is required for:\n%s' "${unformatted_files}" >&2
  exit 1
fi

AUTH_TEST_DATABASE_URL="" AGENT_OBSERVABILITY_TEST_DATABASE_URL="" go test ./...
go vet ./...
