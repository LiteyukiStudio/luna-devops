#!/usr/bin/env bash

set -euo pipefail

if [[ $# -eq 0 ]]; then
  printf '%s\n' 'at least one job result is required' >&2
  exit 2
fi

for result in "$@"; do
  case "${result}" in
    success|skipped)
      ;;
    *)
      printf 'required CI job finished with result: %s\n' "${result}" >&2
      exit 1
      ;;
  esac
done

printf '%s\n' 'all executed CI jobs succeeded'
