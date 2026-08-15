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

assert_count() {
  file=$1
  pattern=$2
  expected=$3
  message=$4
  actual=$(grep -Ec -- "$pattern" "$file" || true)
  [ "$actual" -eq "$expected" ] || fail "$message (got $actual, want $expected)"
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

assert_not_contains "$default_render" 'VOLUME_TRANSFER_STORE' 'volume transfers must be disabled by default'

volume_transfer_render="$tmp_dir/volume-transfer.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set volumeTransfer.enabled=true \
  --set volumeTransfer.s3.endpoint=https://objects.example.com \
  --set volumeTransfer.s3.bucket=luna-volume-transfers \
  --set volumeTransfer.s3.existingSecret=volume-transfer-s3 \
  --set volumeTransfer.callbackBaseUrl=https://devops.example.com > "$volume_transfer_render"
assert_contains "$volume_transfer_render" '^  VOLUME_TRANSFER_STORE: "s3"$' 'the volume transfer store is missing'
assert_contains "$volume_transfer_render" '^  VOLUME_TRANSFER_JOB_IMAGE: "liteyukistudio/luna-worker:' 'the default volume transfer Job image does not follow the Worker image'
assert_contains "$volume_transfer_render" '^  VOLUME_TRANSFER_SPOOL_DIR: "/tmp/luna-devops-volume-transfer-spool"$' 'the volume transfer spool directory is missing'
assert_contains "$volume_transfer_render" '^  VOLUME_TRANSFER_SPOOL_MAX_BYTES: "2Gi"$' 'the volume transfer spool byte budget is missing'
assert_contains "$volume_transfer_render" '^  VOLUME_TRANSFER_SPOOL_MIN_FREE_BYTES: "1Gi"$' 'the volume transfer spool free-space reserve is missing'
assert_contains "$volume_transfer_render" '^  VOLUME_TRANSFER_SPOOL_ORPHAN_AGE: "24h"$' 'the volume transfer spool orphan age is missing'
assert_count "$volume_transfer_render" '^[[:space:]]+name: volume-transfer-s3$' 4 'the API and Worker must both reference the two volume transfer credential keys'
assert_count "$volume_transfer_render" '^[[:space:]]+key: access-key-id$' 2 'the API and Worker access-key references are incomplete'
assert_count "$volume_transfer_render" '^[[:space:]]+key: secret-access-key$' 2 'the API and Worker secret-key references are incomplete'
assert_not_contains "$volume_transfer_render" '^  VOLUME_TRANSFER_S3_ACCESS_KEY_ID:' 'the access key must not be rendered in the ConfigMap'

if helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set volumeTransfer.enabled=true \
  --set volumeTransfer.s3.endpoint=http://objects.example.com \
  --set volumeTransfer.s3.bucket=luna-volume-transfers \
  --set volumeTransfer.s3.existingSecret=volume-transfer-s3 \
  --set volumeTransfer.callbackBaseUrl=https://devops.example.com > /dev/null 2>&1; then
  fail 'an insecure volume transfer endpoint was accepted'
fi

printf 'Helm render tests passed.\n'
