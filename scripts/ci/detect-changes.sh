#!/usr/bin/env bash

set -euo pipefail

readonly ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

files_from=""
base_ref=""
head_ref=""

usage() {
  cat <<'EOF'
Usage: scripts/ci/detect-changes.sh [--files-from FILE | --base REF --head REF]

Without arguments, the script derives the comparison range from GitHub Actions
environment variables. Results are written to GITHUB_OUTPUT when available.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --files-from)
      files_from="${2:-}"
      shift 2
      ;;
    --base)
      base_ref="${2:-}"
      shift 2
      ;;
    --head)
      head_ref="${2:-}"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      printf 'unknown argument: %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

cd "${ROOT_DIR}"

go_changed=false
web_changed=false
agent_changed=false
docs_changed=false
helm_changed=false
api_image=false
worker_image=false
agent_image=false
probe_image=false

mark_all() {
  go_changed=true
  web_changed=true
  agent_changed=true
  docs_changed=true
  helm_changed=true
  api_image=true
  worker_image=true
  agent_image=true
  probe_image=true
}

mark_go_image() {
  local path="$1"
  case "${path}" in
    cmd/api/*|internal/api/*|internal/webui/*)
      api_image=true
      ;;
    cmd/worker/*|cmd/volume-transfer/*|internal/worker/*|internal/volumetransfer/*)
      worker_image=true
      ;;
    cmd/gateway-traffic-probe/*|internal/gatewayprobe/*)
      probe_image=true
      ;;
    internal/telemetry/*)
      api_image=true
      worker_image=true
      probe_image=true
      ;;
    cmd/*|internal/*)
      # Most shared backend packages are linked by both API and Worker. Keep the
      # conservative pair when a narrower executable boundary is not explicit.
      api_image=true
      worker_image=true
      ;;
  esac
}

force_all=false
case "${GITHUB_REF:-}" in
  refs/tags/v*) force_all=true ;;
esac
if [[ "${GITHUB_EVENT_NAME:-}" == "workflow_dispatch" ]]; then
  force_all=true
fi

changed_files=""
if [[ "${force_all}" == true ]]; then
  mark_all
elif [[ -n "${files_from}" ]]; then
  if [[ ! -f "${files_from}" ]]; then
    printf 'changed-files input does not exist: %s\n' "${files_from}" >&2
    exit 1
  fi
  changed_files="$(sed '/^[[:space:]]*$/d' "${files_from}")"
elif [[ -n "${base_ref}" || -n "${head_ref}" ]]; then
  if [[ -z "${base_ref}" || -z "${head_ref}" ]]; then
    printf '%s\n' '--base and --head must be provided together' >&2
    exit 2
  fi
  changed_files="$(git diff --name-only --diff-filter=ACMRD "${base_ref}...${head_ref}")"
elif [[ "${GITHUB_EVENT_NAME:-}" == "pull_request" ]]; then
  if [[ -z "${GITHUB_BASE_REF:-}" ]]; then
    printf '%s\n' 'GITHUB_BASE_REF is required for pull_request change detection' >&2
    exit 1
  fi
  git fetch --no-tags origin "${GITHUB_BASE_REF}" >/dev/null 2>&1
  changed_files="$(git diff --name-only --diff-filter=ACMRD "origin/${GITHUB_BASE_REF}...HEAD")"
elif [[ "${GITHUB_EVENT_NAME:-}" == "push" ]]; then
  before="${GITHUB_EVENT_BEFORE:-}"
  if [[ -n "${before}" && ! "${before}" =~ ^0+$ ]] && git cat-file -e "${before}^{commit}" 2>/dev/null; then
    changed_files="$(git diff --name-only --diff-filter=ACMRD "${before}..${GITHUB_SHA:-HEAD}")"
  elif git rev-parse HEAD^ >/dev/null 2>&1; then
    changed_files="$(git diff --name-only --diff-filter=ACMRD HEAD^..HEAD)"
  else
    force_all=true
    mark_all
  fi
else
  printf '%s\n' 'unable to infer the change range; running every check' >&2
  force_all=true
  mark_all
fi

if [[ "${force_all}" == false && -z "${changed_files}" ]]; then
  printf '%s\n' 'the comparison produced no paths; running every check' >&2
  mark_all
else
  while IFS= read -r path; do
    [[ -z "${path}" ]] && continue
    printf 'changed: %s\n' "${path}" >&2

    case "${path}" in
      .github/workflows/*|scripts/ci/*|scripts/release-check.sh|.go-version)
        mark_all
        continue
        ;;
      go.mod|go.sum)
        go_changed=true
        api_image=true
        worker_image=true
        probe_image=true
        continue
        ;;
      Dockerfile|Dockerfile.api|Dockerfile.worker|Dockerfile.web)
        go_changed=true
        web_changed=true
        api_image=true
        worker_image=true
        probe_image=true
        continue
        ;;
      Dockerfile.agent|luna-agent/Dockerfile)
        agent_changed=true
        agent_image=true
        continue
        ;;
      migrations/*)
        go_changed=true
        api_image=true
        worker_image=true
        continue
        ;;
      openapi/*)
        go_changed=true
        web_changed=true
        agent_changed=true
        api_image=true
        agent_image=true
        continue
        ;;
      packages/ai-interaction-card-contract/*)
        web_changed=true
        agent_changed=true
        api_image=true
        agent_image=true
        continue
        ;;
      web/*)
        web_changed=true
        api_image=true
        continue
        ;;
      luna-agent/*)
        agent_changed=true
        agent_image=true
        continue
        ;;
      docs/*)
        docs_changed=true
        continue
        ;;
      charts/*|helm/*)
        helm_changed=true
        continue
        ;;
      scripts/generate-changelog.sh|scripts/generate-changelog.mjs)
        docs_changed=true
        continue
        ;;
      *.go|*/*.go)
        go_changed=true
        if [[ "${path}" != *_test.go ]]; then
          mark_go_image "${path}"
        fi
        continue
        ;;
    esac

    # Root configuration, shared tooling, internal documentation, and any new
    # path without an explicit ownership rule run every gate rather than risk a
    # false skip.
    mark_all
  done <<< "${changed_files}"
fi

exclude_entries=""
append_exclude_entry() {
  local entry="$1"
  if [[ -n "${exclude_entries}" ]]; then
    exclude_entries+=","
  fi
  exclude_entries+="${entry}"
}

if [[ "${api_image}" != true ]]; then
  append_exclude_entry '{"image_key":"api"}'
fi
if [[ "${worker_image}" != true ]]; then
  append_exclude_entry '{"image_key":"worker"}'
fi
if [[ "${agent_image}" != true ]]; then
  append_exclude_entry '{"image_key":"agent"}'
fi
if [[ "${probe_image}" != true ]]; then
  append_exclude_entry '{"image_key":"gateway-traffic-probe"}'
fi

container_changed=false
if [[ "${api_image}" == true || "${worker_image}" == true || "${agent_image}" == true || "${probe_image}" == true ]]; then
  container_changed=true
fi
exclude_json="[${exclude_entries}]"

emit() {
  local key="$1"
  local value="$2"
  if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    printf '%s=%s\n' "${key}" "${value}" >> "${GITHUB_OUTPUT}"
  else
    printf '%s=%s\n' "${key}" "${value}"
  fi
}

emit go "${go_changed}"
emit web "${web_changed}"
emit agent "${agent_changed}"
emit docs "${docs_changed}"
emit helm "${helm_changed}"
emit container "${container_changed}"
emit container_exclude "${exclude_json}"
