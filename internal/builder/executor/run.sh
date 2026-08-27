#!/bin/sh
set -eu

cd /workspace

AUTH_CLONE_URL="$GIT_CLONE_URL"
if [ -n "${GIT_ACCESS_TOKEN:-}" ]; then
  case "$AUTH_CLONE_URL" in
    https://*) AUTH_CLONE_URL="$(printf "%s" "$AUTH_CLONE_URL" | sed "s#https://#https://x-access-token:${GIT_ACCESS_TOKEN}@#")" ;;
  esac
fi

clone_with_retry() {
  attempt=1
  while [ "$attempt" -le 3 ]; do
    rm -rf source
    if [ -n "${SOURCE_BRANCH:-}" ]; then
      if git clone --depth 1 --single-branch --branch "$SOURCE_BRANCH" "$AUTH_CLONE_URL" source; then
        return 0
      fi
    else
      if git clone --depth 1 "$AUTH_CLONE_URL" source; then
        return 0
      fi
    fi
    if [ "$attempt" -eq 3 ]; then
      return 1
    fi
    sleep $((attempt * 2))
    attempt=$((attempt + 1))
  done
}

short_commit() {
  value="$1"
  printf "%s" "$(printf "%.12s" "$value")"
}

json_escape() {
  printf "%s" "$1" | tr '\n\r' '  ' | sed 's/\\/\\\\/g; s/"/\\"/g'
}

b64_line() {
  printf "%s" "$1" | base64 | tr -d "\n"
}

hook_emit_log() {
  hook_id="$1"
  content="$2"
  printf "::luna-devops-hook-log::%s::%s\n" "$hook_id" "$(b64_line "$content")"
}

hook_emit_complete() {
  hook_id="$1"
  succeeded="$2"
  exit_code="$3"
  message="$4"
  printf "::luna-devops-hook-complete::%s::%s::%s::%s\n" "$hook_id" "$succeeded" "$exit_code" "$(b64_line "$message")"
}

run_hook() {
  hook_id="$1"
  meta="/workspace/hooks/${hook_id}.meta"
  script="/workspace/hooks/${hook_id}.sh"
  if [ ! -f "$meta" ] || [ ! -f "$script" ]; then
    return 0
  fi
  HOOK_NAME=""
  HOOK_SHELL="sh"
  HOOK_TIMEOUT_SECONDS="300"
  HOOK_FAILURE_POLICY="fail"
  # shellcheck disable=SC1090
  . "$meta"
  export LITEYUKI_HOOK_RUN_ID="$hook_id"
  export LITEYUKI_HOOK_PHASE="${HOOK_PHASE:-}"
  hook_emit_log "$hook_id" "开始执行 Hook: ${HOOK_NAME:-$hook_id}"
  set +e
  if command -v timeout >/dev/null 2>&1; then
    output="$(timeout "${HOOK_TIMEOUT_SECONDS:-300}" "${HOOK_SHELL:-sh}" "$script" 2>&1)"
    exit_code=$?
  else
    output="$("${HOOK_SHELL:-sh}" "$script" 2>&1)"
    exit_code=$?
  fi
  set -e
  if [ -n "$output" ]; then
    hook_emit_log "$hook_id" "$output"
  fi
  if [ "$exit_code" -eq 0 ]; then
    hook_emit_complete "$hook_id" "true" "$exit_code" "hook succeeded"
    return 0
  fi
  message="hook failed with exit code $exit_code"
  hook_emit_complete "$hook_id" "false" "$exit_code" "$message"
  if [ "${HOOK_FAILURE_POLICY:-fail}" = "ignore" ]; then
    return 0
  fi
  return "$exit_code"
}

run_hooks() {
  hook_phase="$1"
  hook_ids="$2"
  OLDIFS="$IFS"
  IFS=","
  for hook_id in $hook_ids; do
    if [ -z "$hook_id" ]; then
      continue
    fi
    HOOK_PHASE="$hook_phase" run_hook "$hook_id"
  done
  IFS="$OLDIFS"
}

export_luna_devops_build_context() {
  ref_name="${SOURCE_TAG:-$SOURCE_BRANCH}"
  ref_type="branch"
  ref_value=""
  if [ -n "${SOURCE_TAG:-}" ]; then
    ref_type="tag"
    ref_value="refs/tags/$SOURCE_TAG"
  elif [ -n "${SOURCE_BRANCH:-}" ]; then
    ref_value="refs/heads/$SOURCE_BRANCH"
  fi
  export LITEYUKI_GIT_BRANCH="${SOURCE_BRANCH:-}"
  export LITEYUKI_GIT_TAG="${SOURCE_TAG:-}"
  export LITEYUKI_GIT_REF_NAME="$ref_name"
  export LITEYUKI_GIT_REF_TYPE="$ref_type"
  export LITEYUKI_GIT_REF="$ref_value"
  export LITEYUKI_GIT_SHA="${CHECKED_OUT_COMMIT:-${SOURCE_COMMIT:-}}"
  export LITEYUKI_GIT_SHORT_SHA="$(short_commit "$LITEYUKI_GIT_SHA")"
  export LITEYUKI_IMAGE_REF="${IMAGE_REF:-}"
}

export_luna_devops_build_context
run_hooks "prePull" "${PRE_PULL_HOOK_IDS:-}"

clone_with_retry
cd source

if [ -n "${SOURCE_TAG:-}" ]; then git checkout "$SOURCE_TAG"; fi
if [ -n "${SOURCE_BRANCH:-}" ]; then git checkout "$SOURCE_BRANCH"; fi
if [ -n "${SOURCE_COMMIT:-}" ]; then git checkout "$SOURCE_COMMIT"; fi

CHECKED_OUT_COMMIT="$(git rev-parse HEAD)"
SOURCE_AUTHOR_NAME="$(git log -1 --format=%an)"
SOURCE_AUTHOR_EMAIL="$(git log -1 --format=%ae)"
export_luna_devops_build_context
run_hooks "postPull" "${POST_PULL_HOOK_IDS:-}"

sanitize_tag() {
  printf "%s" "$1" \
    | sed -E 's/[^A-Za-z0-9_.-]+/-/g; s/^[.-]+//; s/[.-]+$//' \
    | cut -c 1-128
}

escape_sed_pattern() {
  printf "%s" "$1" | sed 's/[][\/.^$*+?{}()|]/\\&/g'
}

escape_sed_replacement() {
  printf "%s" "$1" | sed 's/[\/&]/\\&/g'
}

replace_token() {
  pattern="$(escape_sed_pattern "$1")"
  replacement="$(escape_sed_replacement "$2")"
  printf "%s" "$3" | sed -E "s/$pattern/$replacement/g"
}

render_template_value() {
  template="$1"
  ref_name="${SOURCE_TAG:-$SOURCE_BRANCH}"
  ref_type="branch"
  ref_value=""
  if [ -n "${SOURCE_TAG:-}" ]; then
    ref_type="tag"
    ref_value="refs/tags/$SOURCE_TAG"
  elif [ -n "${SOURCE_BRANCH:-}" ]; then
    ref_value="refs/heads/$SOURCE_BRANCH"
  fi
  rendered="$template"
  short_sha="$(short_commit "$CHECKED_OUT_COMMIT")"
  rendered="$(replace_token '${{ github.sha }}' "$CHECKED_OUT_COMMIT" "$rendered")"
  rendered="$(replace_token '${{ github.ref_name }}' "$ref_name" "$rendered")"
  rendered="$(replace_token '${{ github.ref_type }}' "$ref_type" "$rendered")"
  rendered="$(replace_token '${{ github.ref }}' "$ref_value" "$rendered")"
  rendered="$(replace_token '{short_sha}' "$short_sha" "$rendered")"
  printf "%s" "$rendered"
}

render_image_tag() {
  rendered="$(render_template_value "${IMAGE_TAG_TEMPLATE:-latest}")"
  sanitized="$(sanitize_tag "$rendered")"
  if [ -z "$sanitized" ]; then
    sanitized="latest"
  fi
  printf "%s" "$sanitized"
}

if [ -n "${IMAGE_NAME_PREFIX:-}" ]; then
  IMAGE_REF="${IMAGE_NAME_PREFIX}:$(render_image_tag)"
fi
export_luna_devops_build_context

run_hooks "preBuild" "${PRE_BUILD_HOOK_IDS:-}"

set --
OLDIFS="$IFS"
IFS=","
for key in ${BUILD_ENV_KEYS:-}; do
  if [ -z "$key" ]; then
    continue
  fi
  eval "value=\${$key:-}"
  set -- "$@" --opt "build-arg:${key}=${value}"
done
for key in ${BUILD_ARG_KEYS:-}; do
  if [ -z "$key" ]; then
    continue
  fi
  eval "value=\${$key:-}"
  value="$(render_template_value "$value")"
  set -- "$@" --opt "build-arg:${key}=${value}"
done
IFS="$OLDIFS"

if [ "${CACHE_ENABLED:-false}" = "true" ]; then
  cache_tag="${CACHE_TAG:-buildcache}"
  cache_repository="${IMAGE_REF%:*}"
  if [ -n "$cache_repository" ] && [ "$cache_repository" != "$IMAGE_REF" ]; then
    cache_ref="${cache_repository}:$cache_tag"
    set -- "$@" --import-cache "type=registry,ref=$cache_ref"
    set -- "$@" --export-cache "type=registry,ref=$cache_ref,mode=max"
  fi
fi

mkdir -p "$HOME/.docker"
AUTH="$(printf "%s:%s" "$REGISTRY_USERNAME" "$REGISTRY_PASSWORD" | base64 | tr -d "\n")"
printf '{"auths":{"%s":{"auth":"%s"}}}' "$REGISTRY_ENDPOINT" "$AUTH" > "$HOME/.docker/config.json"

run_build() {
	dockerfile_dir="$PWD/$(dirname "$DOCKERFILE_PATH")"
	dockerfile_name="$(basename "$DOCKERFILE_PATH")"
	if [ "${BUILD_DEFINITION_MODE:-repository_dockerfile}" = "template" ]; then
		if [ ! -s /executor/template.Dockerfile ]; then
			echo "platform build template snapshot is unavailable" >&2
			return 1
		fi
		dockerfile_dir="/executor"
		dockerfile_name="template.Dockerfile"
	fi
  buildctl-daemonless.sh build \
    --progress=plain \
    --frontend dockerfile.v0 \
    --local context="$PWD/$BUILD_CONTEXT" \
    --local dockerfile="$dockerfile_dir" \
    --opt filename="$dockerfile_name" \
    "$@" \
    --output type=image,name="$IMAGE_REF",push=true
}

run_hooks "prePush" "${PRE_PUSH_HOOK_IDS:-}"
run_build "$@"

export_luna_devops_build_context
run_hooks "postPush" "${POST_PUSH_HOOK_IDS:-}"
run_hooks "postBuild" "${POST_BUILD_HOOK_IDS:-}"

RESULT_JSON="$(printf '{"imageRef":"%s","sourceCommit":"%s","sourceAuthorName":"%s","sourceAuthorEmail":"%s","message":"builder task succeeded"}' "$(json_escape "$IMAGE_REF")" "$(json_escape "$CHECKED_OUT_COMMIT")" "$(json_escape "$SOURCE_AUTHOR_NAME")" "$(json_escape "$SOURCE_AUTHOR_EMAIL")")"
printf "%s" "$RESULT_JSON" > /workspace/result.json
printf "::luna-devops-build-result::%s\n" "$(b64_line "$RESULT_JSON")"
