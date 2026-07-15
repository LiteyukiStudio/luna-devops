#!/bin/sh
set -eu

chart_dir=${1:-"$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"}
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

fail() {
  printf 'render test failed: %s\n' "$1" >&2
  exit 1
}

assert_contains() {
  file=$1
  pattern=$2
  message=$3
  grep -Eq -- "$pattern" "$file" || fail "$message"
}

assert_not_contains() {
  file=$1
  pattern=$2
  message=$3
  if grep -Eq -- "$pattern" "$file"; then
    fail "$message"
  fi
}

default_render="$tmp_dir/default.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops > "$default_render"
assert_contains "$default_render" '^  redis-url: "redis://default:[[:alnum:]]{32}@luna-devops-redis:6379/0"$' 'the built-in Redis URI was not generated'
assert_contains "$default_render" '^  redis-password: "[[:alnum:]]{32}"$' 'the built-in Redis password was not generated'
assert_contains "$default_render" '^[[:space:]]+- name: REDIS_ADDR$' 'REDIS_ADDR is missing'
assert_contains "$default_render" '^[[:space:]]+- name: REDIS_PASSWORD$' 'the built-in Redis password environment variable is missing'
assert_contains "$default_render" 'requirepass "\$REDIS_PASSWORD"' 'the built-in Redis server does not require authentication'
redis_password=$(sed -n 's/^  redis-password: "\([[:alnum:]]\{32\}\)"$/\1/p' "$default_render")
redis_url_password=$(sed -n 's#^  redis-url: "redis://default:\([[:alnum:]]\{32\}\)@luna-devops-redis:6379/0"$#\1#p' "$default_render")
[ "$redis_password" = "$redis_url_password" ] || fail 'the built-in Redis password and URI do not match'

internal_secret_render="$tmp_dir/internal-existing-secret.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set redis.auth.existingSecret=redis-auth > "$internal_secret_render"
assert_contains "$internal_secret_render" '^[[:space:]]+name: redis-auth$' 'the built-in Redis existingSecret is not referenced'
assert_contains "$internal_secret_render" '^[[:space:]]+key: redis-url$' 'the built-in Redis URL key is not referenced'
assert_contains "$internal_secret_render" '^[[:space:]]+key: redis-password$' 'the built-in Redis password key is not referenced'
assert_not_contains "$internal_secret_render" '^  redis-url:' 'a Redis URI must not be generated when redis.auth.existingSecret is set'

external_secret_render="$tmp_dir/external-existing-secret.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set redis.enabled=false \
  --set externalRedis.existingSecret=external-redis \
  --set externalDatabase.url='postgres://user:password@postgres:5432/devops' > "$external_secret_render"
assert_contains "$external_secret_render" '^[[:space:]]+name: external-redis$' 'the external Redis Secret is not referenced'
assert_contains "$external_secret_render" '^[[:space:]]+key: redis-url$' 'the external Redis URI key is not referenced'
assert_not_contains "$external_secret_render" '^[[:space:]]+- name: REDIS_(USERNAME|PASSWORD|DB)$' 'split Redis environment variables are still rendered'

printf 'Redis Helm render tests passed.\n'
