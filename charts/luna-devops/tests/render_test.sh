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

assert_contains "$default_render" '^  VOLUME_TRANSFER_MAX_BYTES: "100Gi"$' 'the direct transfer size limit is missing'
assert_contains "$default_render" '^  VOLUME_TRANSFER_JOB_IMAGE: "liteyukistudio/luna-worker:' 'the direct transfer image does not follow the Worker image'
assert_not_contains "$default_render" 'VOLUME_TRANSFER_(STORE|S3|SPOOL|CALLBACK|OBJECT_TTL)' 'object-store transfer settings are still rendered'
assert_count "$default_render" '^[[:space:]]+- name: LOG_FORMAT$' 2 'API and Worker LOG_FORMAT settings are missing'
assert_count "$default_render" '^[[:space:]]+value: "json"$' 2 'production logs are not fixed to JSON'
assert_count "$default_render" '^[[:space:]]+- name: LOG_COLOR$' 2 'API and Worker LOG_COLOR settings are missing'

agent_render="$tmp_dir/agent.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set ai.enabled=true \
  --set ai.existingSecret=agent-auth > "$agent_render"
assert_count "$agent_render" '^[[:space:]]+- name: LOG_FORMAT$' 3 'Agent LOG_FORMAT does not match API and Worker'
assert_count "$agent_render" '^[[:space:]]+- name: LOG_LEVEL$' 3 'Agent LOG_LEVEL does not match API and Worker'

agent_network_default="$tmp_dir/agent-network-default.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set ai.enabled=true \
  --set ai.existingSecret=agent-auth \
  --show-only templates/ai-networkpolicy.yaml > "$agent_network_default"
assert_count "$agent_network_default" '^kind: NetworkPolicy$' 1 'Agent default network isolation should only render ingress'
assert_contains "$agent_network_default" '^  name: luna-devops-agent-ingress$' 'Agent ingress policy is missing'
assert_contains "$agent_network_default" '^[[:space:]]+- Ingress$' 'Agent ingress policy type is missing'
assert_not_contains "$agent_network_default" '^[[:space:]]+- Egress$' 'Agent egress isolation must require explicit opt-in'

agent_egress_values="$tmp_dir/agent-egress-values.yaml"
cat > "$agent_egress_values" <<'EOF'
ai:
  enabled: true
  existingSecret: agent-auth
  agent:
    networkPolicy:
      ingress:
        enabled: true
      egress:
        enabled: true
        additionalCIDRs:
          - 203.0.113.10/32
        additionalRules:
          - to:
              - namespaceSelector:
                  matchLabels:
                    kubernetes.io/metadata.name: observability
            ports:
              - protocol: TCP
                port: 4318
EOF
agent_network_egress="$tmp_dir/agent-network-egress.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops \
  --values "$agent_egress_values" \
  --show-only templates/ai-networkpolicy.yaml > "$agent_network_egress"
assert_count "$agent_network_egress" '^kind: NetworkPolicy$' 2 'Agent ingress and opted-in egress policies were not split'
assert_contains "$agent_network_egress" '^  name: luna-devops-agent-egress$' 'Agent egress policy is missing after opt-in'
assert_contains "$agent_network_egress" 'cidr: "203\.0\.113\.10/32"' 'Agent additional egress CIDR is missing'
assert_contains "$agent_network_egress" 'port: 4318' 'Agent additional egress rule is missing'
assert_not_contains "$agent_network_egress" 'port: 6379' 'Agent no longer uses Redis egress'

agent_deployment_render="$tmp_dir/agent-deployment.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set ai.enabled=true \
  --set ai.existingSecret=agent-auth \
  --show-only templates/ai-agent.yaml > "$agent_deployment_render"
assert_contains "$agent_deployment_render" '^[[:space:]]+replicas: 1$' 'Agent must remain a single replica'
assert_contains "$agent_deployment_render" '^[[:space:]]+type: Recreate$' 'Agent deployment must avoid overlapping owners during rollout'
assert_not_contains "$agent_deployment_render" '^[[:space:]]+- name: REDIS_ADDR$' 'Agent must not receive REDIS_ADDR'
assert_not_contains "$agent_deployment_render" '^kind: PodDisruptionBudget$' 'single-replica Agent must not render a disruption budget'
assert_contains "$agent_deployment_render" '^[[:space:]]+automountServiceAccountToken: false$' 'Agent ServiceAccount token mounting must remain disabled'
assert_contains "$agent_deployment_render" '^[[:space:]]+readOnlyRootFilesystem: true$' 'Agent root filesystem must remain read-only'

printf 'Helm render tests passed.\n'
