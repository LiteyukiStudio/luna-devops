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

assert_env_secret_optional() {
  file=$1
  env_name=$2
  expected_refs=$3
  expected_optional=$4
  result=$(awk -v env_name="$env_name" '
    /^[[:space:]]+- name: / {
      if (active) {
        refs++
        optional += block_optional
      }
      active = ($3 == env_name)
      block_optional = 0
      next
    }
    active && /^[[:space:]]+optional: true$/ { block_optional = 1 }
    END {
      if (active) {
        refs++
        optional += block_optional
      }
      printf "%d:%d", refs, optional
    }
  ' "$file")
  [ "$result" = "$expected_refs:$expected_optional" ] || \
    fail "$env_name Secret refs expected $expected_refs:$expected_optional, got $result"
}

default_render="$tmp_dir/default.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set redis.auth.username=must-be-ignored > "$default_render"
assert_contains "$default_render" '^  redis-password: "[[:alnum:]]{32}"$' 'the built-in Redis password was not generated'
assert_contains "$default_render" '^[[:space:]]+- name: REDIS_USERNAME$' 'REDIS_USERNAME is missing'
assert_contains "$default_render" '^[[:space:]]+value: "default"$' 'the built-in Redis username is not fixed to default'
assert_not_contains "$default_render" 'must-be-ignored' 'the built-in Redis username can still be overridden'
assert_contains "$default_render" 'requirepass "\$REDIS_PASSWORD"' 'the built-in Redis server does not require authentication'
assert_not_contains "$default_render" '^  redis-username:' 'the built-in Redis username must not be configurable through a generated Secret'

internal_secret_render="$tmp_dir/internal-existing-secret.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set redis.auth.existingSecret=redis-auth > "$internal_secret_render"
assert_contains "$internal_secret_render" '^[[:space:]]+name: redis-auth$' 'the built-in Redis existingSecret is not referenced'
assert_contains "$internal_secret_render" '^[[:space:]]+key: redis-password$' 'the built-in Redis password key is not referenced'
assert_not_contains "$internal_secret_render" '^  redis-password:' 'a password must not be generated when redis.auth.existingSecret is set'
assert_contains "$internal_secret_render" '^[[:space:]]+value: "default"$' 'the built-in Redis existingSecret path changed the fixed username'

external_secret_render="$tmp_dir/external-existing-secret.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set redis.enabled=false \
  --set externalRedis.existingSecret=external-redis \
  --set externalDatabase.url='postgres://user:password@postgres:5432/devops' > "$external_secret_render"
assert_contains "$external_secret_render" '^[[:space:]]+name: external-redis$' 'the external Redis Secret is not referenced'
assert_contains "$external_secret_render" '^[[:space:]]+key: redis-password$' 'the external Redis password key is not required'
assert_env_secret_optional "$external_secret_render" REDIS_USERNAME 2 2
assert_env_secret_optional "$external_secret_render" REDIS_DB 2 2
assert_env_secret_optional "$external_secret_render" REDIS_PASSWORD 2 0

printf 'Redis Helm render tests passed.\n'
