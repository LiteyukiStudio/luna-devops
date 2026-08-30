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

short_password_error="$tmp_dir/short-initial-admin-password.err"
if helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set-string api.initialAdmin.email=admin@example.com \
  --set-string api.initialAdmin.password=short > /dev/null 2> "$short_password_error"; then
  fail 'a chart-managed initial administrator password shorter than eight bytes must fail'
fi
assert_contains "$short_password_error" 'api\.initialAdmin\.password must contain 8 to 72 bytes' 'the initial administrator password constraint is not enforced'

invalid_language_error="$tmp_dir/invalid-initial-admin-language.err"
if helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set-string api.initialAdmin.email=admin@example.com \
  --set-string api.initialAdmin.password=correct-horse-battery-staple \
  --set-string api.initialAdmin.language=fr-FR > /dev/null 2> "$invalid_language_error"; then
  fail 'an unsupported initial administrator language must fail'
fi
assert_contains "$invalid_language_error" 'api\.initialAdmin\.language must be zh-CN or en-US' 'the initial administrator language constraint is not enforced'

legacy_extra_env_error="$tmp_dir/legacy-extra-env.err"
if helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set-string app.extraEnv.LEGACY_VALUE=must-migrate > /dev/null 2> "$legacy_extra_env_error"; then
  fail 'the removed shared app.extraEnv must fail'
fi
assert_contains "$legacy_extra_env_error" 'app.extraEnv has been removed' 'shared app.extraEnv does not require workload-scoped migration'

api_managed_env_error="$tmp_dir/api-managed-env.err"
if helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set-string api.extraEnv.DATABASE_URL=postgres://must-not-override > /dev/null 2> "$api_managed_env_error"; then
  fail 'api.extraEnv must not override a managed Secret-backed variable'
fi
assert_contains "$api_managed_env_error" 'DATABASE_URL is managed by the chart and cannot be set through api.extraEnv' 'api.extraEnv does not reject managed variables'

api_browser_trace_headers_error="$tmp_dir/api-browser-trace-headers.err"
if helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set-string api.extraEnv.OTEL_EXPORTER_OTLP_TRACES_HEADERS='Authorization=Bearer%20plaintext' > /dev/null 2> "$api_browser_trace_headers_error"; then
  fail 'api.extraEnv must not accept browser Trace Relay credentials'
fi
assert_contains "$api_browser_trace_headers_error" 'OTEL_EXPORTER_OTLP_TRACES_HEADERS is managed by the chart and cannot be set through api.extraEnv' 'browser Trace Relay credentials can still be stored in plaintext values'

worker_secret_env_error="$tmp_dir/worker-secret-env.err"
if helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set-string worker.extraEnv.AI_INTERNAL_SECRET=must-not-cross-boundary > /dev/null 2> "$worker_secret_env_error"; then
  fail 'worker.extraEnv must not receive the API-Agent internal secret'
fi
assert_contains "$worker_secret_env_error" 'AI_INTERNAL_SECRET is managed by the chart and cannot be set through worker.extraEnv' 'worker.extraEnv does not reject managed secrets'

api_worker_env_error="$tmp_dir/api-worker-env.err"
if helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set-string api.extraEnv.BUILD_EGRESS_MODE=permissive > /dev/null 2> "$api_worker_env_error"; then
  fail 'api.extraEnv must not receive Worker-managed configuration'
fi
assert_contains "$api_worker_env_error" 'BUILD_EGRESS_MODE is managed by the chart and cannot be set through api.extraEnv' 'API accepts Worker-managed variables through extraEnv'

worker_api_env_error="$tmp_dir/worker-api-env.err"
if helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set-string worker.extraEnv.API_ADDR=:9090 > /dev/null 2> "$worker_api_env_error"; then
  fail 'worker.extraEnv must not receive API-managed configuration'
fi
assert_contains "$worker_api_env_error" 'API_ADDR is managed by the chart and cannot be set through worker.extraEnv' 'Worker accepts API-managed variables through extraEnv'

api_database_env_error="$tmp_dir/api-database-env.err"
if helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set-string api.extraEnv.WORKER_DB_MAX_OPEN_CONNS=99 > /dev/null 2> "$api_database_env_error"; then
  fail 'api.extraEnv must not receive Worker database-pool configuration'
fi
assert_contains "$api_database_env_error" 'WORKER_DB_MAX_OPEN_CONNS is managed by the chart and cannot be set through api.extraEnv' 'API accepts Worker database-pool variables through extraEnv'

worker_database_env_error="$tmp_dir/worker-database-env.err"
if helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set-string worker.extraEnv.API_DB_MAX_OPEN_CONNS=99 > /dev/null 2> "$worker_database_env_error"; then
  fail 'worker.extraEnv must not receive API database-pool configuration'
fi
assert_contains "$worker_database_env_error" 'API_DB_MAX_OPEN_CONNS is managed by the chart and cannot be set through worker.extraEnv' 'Worker accepts API database-pool variables through extraEnv'

initial_admin_values="$tmp_dir/initial-admin.yaml"
cat > "$initial_admin_values" <<'EOF'
api:
  initialAdmin:
    email: admin@example.com
    name: Platform Admin
    password: correct-horse-battery-staple
    language: en-US
EOF

default_render="$tmp_dir/default.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops > "$default_render"
assert_not_contains "$default_render" '^  name: luna-devops-initial-admin$' 'an unused chart-managed initial administrator Secret was rendered'
for initial_admin_env in INITIAL_ADMIN_EMAIL INITIAL_ADMIN_NAME INITIAL_ADMIN_PASSWORD INITIAL_ADMIN_LANGUAGE; do
  assert_count "$default_render" "^[[:space:]]+- name: ${initial_admin_env}$" 1 "$initial_admin_env must only be injected into API"
done
assert_count "$default_render" '^[[:space:]]+optional: true$' 4 'all initial administrator Secret references must be optional'
assert_not_contains "$default_render" 'BOOTSTRAP_TOKEN' 'the removed bootstrap token is still rendered'
assert_not_contains "$default_render" 'OTEL_EXPORTER_OTLP_TRACES_HEADERS' 'browser Trace Relay headers were rendered without an existing Secret'
assert_count "$default_render" '^kind: ConfigMap$' 3 'shared, API, and Worker ConfigMaps were not rendered separately'
assert_contains "$default_render" '^  AI_AGENT_BASE_URL:' 'the canonical Agent base URL is missing'
assert_not_contains "$default_render" 'AI_AGENT_ADDR' 'the legacy Agent address variable is still rendered'
assert_contains "$default_render" '^  redis-url: "redis://default:[[:alnum:]]{32}@luna-devops-redis:6379/0"$' 'the built-in Redis URI was not generated'
assert_contains "$default_render" '^  redis-password: "[[:alnum:]]{32}"$' 'the built-in Redis password was not generated'
assert_contains "$default_render" '^[[:space:]]+- name: REDIS_ADDR$' 'REDIS_ADDR is missing'
assert_contains "$default_render" '^[[:space:]]+- name: REDIS_PASSWORD$' 'the built-in Redis password environment variable is missing'
assert_contains "$default_render" 'requirepass "\$REDIS_PASSWORD"' 'the built-in Redis server does not require authentication'
redis_password=$(sed -n 's/^  redis-password: "\([[:alnum:]]\{32\}\)"$/\1/p' "$default_render")
redis_url_password=$(sed -n 's#^  redis-url: "redis://default:\([[:alnum:]]\{32\}\)@luna-devops-redis:6379/0"$#\1#p' "$default_render")
[ "$redis_password" = "$redis_url_password" ] || fail 'the built-in Redis password and URI do not match'

managed_admin_render="$tmp_dir/managed-initial-admin.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops \
  --values "$initial_admin_values" > "$managed_admin_render"
assert_contains "$managed_admin_render" '^  name: luna-devops-initial-admin$' 'the requested chart-managed initial administrator Secret is missing'
assert_contains "$managed_admin_render" '^  initial-admin-email: "admin@example\.com"$' 'the initial administrator email is missing'
assert_contains "$managed_admin_render" '^  initial-admin-name: "Platform Admin"$' 'the initial administrator name is missing'
assert_contains "$managed_admin_render" '^  initial-admin-password: "correct-horse-battery-staple"$' 'the initial administrator password is missing'
assert_contains "$managed_admin_render" '^  initial-admin-language: "en-US"$' 'the initial administrator language is missing'

partial_admin_render="$tmp_dir/partial-initial-admin.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set-string api.initialAdmin.name='Platform Admin' \
  --show-only templates/secret.yaml > "$partial_admin_render"
assert_contains "$partial_admin_render" '^  name: luna-devops-initial-admin$' 'an explicitly provided optional administrator field did not create the managed Secret'
assert_contains "$partial_admin_render" '^  initial-admin-name: "Platform Admin"$' 'the provided optional administrator name is missing'
assert_not_contains "$partial_admin_render" '^  initial-admin-(email|password|language):' 'unset initial administrator keys must be omitted from the managed Secret'

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

external_admin_render="$tmp_dir/external-initial-admin.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set api.initialAdmin.existingSecret=platform-initial-admin > "$external_admin_render"
assert_not_contains "$external_admin_render" '^  name: luna-devops-initial-admin$' 'a chart-managed initial administrator Secret was rendered with existingSecret'
assert_count "$external_admin_render" '^[[:space:]]+name: platform-initial-admin$' 4 'the external initial administrator Secret must back all four API variables'
assert_count "$external_admin_render" '^[[:space:]]+key: initial-admin-(email|name|password|language)$' 4 'the external initial administrator Secret keys are incomplete'
assert_count "$external_admin_render" '^[[:space:]]+optional: true$' 4 'external initial administrator Secret keys must remain optional'

notes_render="$tmp_dir/notes.txt"
helm install luna-devops-notes "$chart_dir" --namespace luna-devops \
  --values "$initial_admin_values" \
  --dry-run=client \
  --hide-secret > "$notes_render"
assert_contains "$notes_render" 'http://localhost:8088/login' 'the install notes do not point to the login page'
assert_not_contains "$notes_render" 'correct-horse-battery-staple' 'the install notes expose the initial administrator password'

assert_contains "$default_render" '^  VOLUME_TRANSFER_MAX_BYTES: "100Gi"$' 'the direct transfer size limit is missing'
assert_contains "$default_render" '^  VOLUME_TRANSFER_JOB_IMAGE: "liteyukistudio/luna-worker:' 'the direct transfer image does not follow the Worker image'
assert_not_contains "$default_render" 'VOLUME_TRANSFER_(STORE|S3|SPOOL|CALLBACK|OBJECT_TTL)' 'object-store transfer settings are still rendered'
assert_count "$default_render" '^[[:space:]]+- name: LOG_FORMAT$' 2 'API and Worker LOG_FORMAT settings are missing'
assert_count "$default_render" '^[[:space:]]+value: "json"$' 2 'production logs are not fixed to JSON'
assert_count "$default_render" '^[[:space:]]+- name: LOG_COLOR$' 2 'API and Worker LOG_COLOR settings are missing'
assert_count "$default_render" '^[[:space:]]+checksum\.luna\.devops/shared-config:' 2 'API and Worker shared ConfigMap checksums are missing'
assert_count "$default_render" '^[[:space:]]+checksum\.luna\.devops/managed-secrets:' 4 'managed Secret checksums are missing from API, Worker, PostgreSQL, or Redis'

shared_config_render="$tmp_dir/shared-config.yaml"
api_config_render="$tmp_dir/api-config.yaml"
worker_config_render="$tmp_dir/worker-config.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops --show-only templates/configmap.yaml > "$shared_config_render"
helm template luna-devops "$chart_dir" --namespace luna-devops --show-only templates/api-configmap.yaml > "$api_config_render"
helm template luna-devops "$chart_dir" --namespace luna-devops --show-only templates/worker-configmap.yaml > "$worker_config_render"
assert_contains "$shared_config_render" '^  PUBLIC_BASE_URL:' 'PUBLIC_BASE_URL is missing from shared configuration'
assert_contains "$shared_config_render" '^  VOLUME_TRANSFER_MAX_BYTES:' 'the shared transfer limit is missing'
assert_not_contains "$shared_config_render" 'API_ADDR|APP_CORS_ORIGINS|BUILD_EXECUTOR_IMAGE|AI_AGENT_BASE_URL' 'workload-only settings leaked into shared configuration'
assert_contains "$api_config_render" '^  API_ADDR:' 'API_ADDR is missing from API configuration'
assert_contains "$api_config_render" '^  API_DB_MAX_OPEN_CONNS: "20"$' 'the API database-pool size is missing'
assert_contains "$api_config_render" '^  API_DB_CONN_MAX_LIFETIME: "30m"$' 'the API database-pool lifetime is missing'
assert_contains "$api_config_render" '^  AI_AGENT_BASE_URL:' 'the Agent URL is missing from API configuration'
assert_not_contains "$api_config_render" 'WORKER_DB_|BUILD_EXECUTOR_IMAGE|BUILD_EGRESS_MODE|DEPLOY_ROLLOUT_TIMEOUT_SECONDS' 'Worker settings leaked into API configuration'
assert_contains "$worker_config_render" '^  WORKER_DB_MAX_OPEN_CONNS: "20"$' 'the Worker database-pool size is missing'
assert_contains "$worker_config_render" '^  WORKER_DB_CONN_MAX_LIFETIME: "30m"$' 'the Worker database-pool lifetime is missing'
assert_contains "$worker_config_render" '^  BUILD_EXECUTOR_IMAGE:' 'the build executor is missing from Worker configuration'
assert_contains "$worker_config_render" '^  BUILD_EGRESS_MODE:' 'the build egress mode is missing from Worker configuration'
assert_not_contains "$worker_config_render" 'API_DB_|API_ADDR|APP_CORS_ORIGINS|TRUSTED_PROXY_CIDRS|AI_AGENT_' 'API settings leaked into Worker configuration'

custom_database_render="$tmp_dir/custom-database-config.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set api.database.maxOpenConns=31 \
  --set api.database.connMaxIdleTime=90s \
  --set worker.database.maxOpenConns=17 \
  --set worker.database.connMaxIdleTime=45s > "$custom_database_render"
assert_contains "$custom_database_render" '^  API_DB_MAX_OPEN_CONNS: "31"$' 'custom API database-pool size was not rendered'
assert_contains "$custom_database_render" '^  API_DB_CONN_MAX_IDLE_TIME: "90s"$' 'custom API database-pool idle time was not rendered'
assert_contains "$custom_database_render" '^  WORKER_DB_MAX_OPEN_CONNS: "17"$' 'custom Worker database-pool size was not rendered'
assert_contains "$custom_database_render" '^  WORKER_DB_CONN_MAX_IDLE_TIME: "45s"$' 'custom Worker database-pool idle time was not rendered'

api_extra_env_render="$tmp_dir/api-extra-env.yaml"
worker_extra_env_render="$tmp_dir/worker-extra-env.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set-string api.extraEnv.API_FEATURE_FLAG=enabled \
  --show-only templates/api-deployment.yaml > "$api_extra_env_render"
helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set-string worker.extraEnv.WORKER_FEATURE_FLAG=enabled \
  --show-only templates/worker-deployment.yaml > "$worker_extra_env_render"
assert_contains "$api_extra_env_render" '^[[:space:]]+- name: "API_FEATURE_FLAG"$' 'API-scoped extraEnv is missing from API'
assert_not_contains "$api_extra_env_render" 'WORKER_FEATURE_FLAG' 'Worker extraEnv leaked into API'
assert_contains "$worker_extra_env_render" '^[[:space:]]+- name: "WORKER_FEATURE_FLAG"$' 'Worker-scoped extraEnv is missing from Worker'
assert_not_contains "$worker_extra_env_render" 'API_FEATURE_FLAG' 'API extraEnv leaked into Worker'

browser_trace_render="$tmp_dir/browser-trace-observability.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set ai.enabled=true \
  --set ai.existingSecret=agent-auth \
  --set observability.otlpEndpoint=http://otel-collector.observability.svc.cluster.local:4318 \
  --set observability.existingSecret=collector-auth \
  --set observability.headersKey=otlp-headers \
  --set api.browserTrace.existingSecret=browser-trace-auth \
  --set api.browserTrace.headersKey=browser-trace-headers \
  --set-string api.extraEnv.OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=https://browser-trace.example.com/v1/traces > "$browser_trace_render"
assert_count "$browser_trace_render" '^[[:space:]]+- name: OTEL_EXPORTER_OTLP_HEADERS$' 3 'the shared OTLP Secret must continue to reach API, Worker, and Agent'
assert_count "$browser_trace_render" '^[[:space:]]+name: collector-auth$' 3 'the shared OTLP Secret reference changed while browser Trace authentication was enabled'
assert_count "$browser_trace_render" '^[[:space:]]+- name: OTEL_EXPORTER_OTLP_TRACES_HEADERS$' 1 'browser Trace Relay headers must only be injected into API'
assert_count "$browser_trace_render" '^[[:space:]]+name: browser-trace-auth$' 1 'the API-only browser Trace Secret reference is missing or leaked to another workload'
assert_contains "$browser_trace_render" '^[[:space:]]+key: browser-trace-headers$' 'the configured browser Trace Secret key is missing'
assert_count "$browser_trace_render" '^[[:space:]]+- name: "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"$' 1 'the non-sensitive browser Trace endpoint must remain API-scoped'

stable_rollout_values="$tmp_dir/stable-rollout-values.yaml"
cat > "$stable_rollout_values" <<'EOF'
app:
  existingSecret: app-secret
postgresql:
  enabled: false
externalDatabase:
  existingSecret: database-secret
redis:
  enabled: false
externalRedis:
  existingSecret: redis-secret
EOF

api_worker_config_before="$tmp_dir/api-worker-config-before.yaml"
api_worker_config_after="$tmp_dir/api-worker-config-after.yaml"
worker_worker_config_before="$tmp_dir/worker-worker-config-before.yaml"
worker_worker_config_after="$tmp_dir/worker-worker-config-after.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops --values "$stable_rollout_values" \
  --set worker.buildEgressMode=restricted --show-only templates/api-deployment.yaml > "$api_worker_config_before"
helm template luna-devops "$chart_dir" --namespace luna-devops --values "$stable_rollout_values" \
  --set worker.buildEgressMode=permissive --show-only templates/api-deployment.yaml > "$api_worker_config_after"
helm template luna-devops "$chart_dir" --namespace luna-devops --values "$stable_rollout_values" \
  --set worker.buildEgressMode=restricted --show-only templates/worker-deployment.yaml > "$worker_worker_config_before"
helm template luna-devops "$chart_dir" --namespace luna-devops --values "$stable_rollout_values" \
  --set worker.buildEgressMode=permissive --show-only templates/worker-deployment.yaml > "$worker_worker_config_after"
cmp -s "$api_worker_config_before" "$api_worker_config_after" || fail 'a Worker-only configuration change restarted API'
if cmp -s "$worker_worker_config_before" "$worker_worker_config_after"; then
  fail 'a Worker configuration change did not update the Worker pod template checksum'
fi

api_database_before="$tmp_dir/api-database-before.yaml"
api_database_after="$tmp_dir/api-database-after.yaml"
worker_api_database_before="$tmp_dir/worker-api-database-before.yaml"
worker_api_database_after="$tmp_dir/worker-api-database-after.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops --values "$stable_rollout_values" \
  --set api.database.maxOpenConns=20 --show-only templates/api-deployment.yaml > "$api_database_before"
helm template luna-devops "$chart_dir" --namespace luna-devops --values "$stable_rollout_values" \
  --set api.database.maxOpenConns=21 --show-only templates/api-deployment.yaml > "$api_database_after"
helm template luna-devops "$chart_dir" --namespace luna-devops --values "$stable_rollout_values" \
  --set api.database.maxOpenConns=20 --show-only templates/worker-deployment.yaml > "$worker_api_database_before"
helm template luna-devops "$chart_dir" --namespace luna-devops --values "$stable_rollout_values" \
  --set api.database.maxOpenConns=21 --show-only templates/worker-deployment.yaml > "$worker_api_database_after"
if cmp -s "$api_database_before" "$api_database_after"; then
  fail 'an API database-pool change did not update the API pod template checksum'
fi
cmp -s "$worker_api_database_before" "$worker_api_database_after" || fail 'an API database-pool change restarted Worker'

api_shared_before="$tmp_dir/api-shared-before.yaml"
api_shared_after="$tmp_dir/api-shared-after.yaml"
worker_shared_before="$tmp_dir/worker-shared-before.yaml"
worker_shared_after="$tmp_dir/worker-shared-after.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops --values "$stable_rollout_values" \
  --set app.publicBaseUrl=https://before.example --show-only templates/api-deployment.yaml > "$api_shared_before"
helm template luna-devops "$chart_dir" --namespace luna-devops --values "$stable_rollout_values" \
  --set app.publicBaseUrl=https://after.example --show-only templates/api-deployment.yaml > "$api_shared_after"
helm template luna-devops "$chart_dir" --namespace luna-devops --values "$stable_rollout_values" \
  --set app.publicBaseUrl=https://before.example --show-only templates/worker-deployment.yaml > "$worker_shared_before"
helm template luna-devops "$chart_dir" --namespace luna-devops --values "$stable_rollout_values" \
  --set app.publicBaseUrl=https://after.example --show-only templates/worker-deployment.yaml > "$worker_shared_after"
if cmp -s "$api_shared_before" "$api_shared_after"; then
  fail 'a shared public URL change did not update the API pod template checksum'
fi
if cmp -s "$worker_shared_before" "$worker_shared_after"; then
  fail 'a shared public URL change did not update the Worker pod template checksum'
fi

agent_render="$tmp_dir/agent.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set ai.enabled=true \
  --set ai.existingSecret=agent-auth > "$agent_render"
assert_count "$agent_render" '^[[:space:]]+- name: LOG_FORMAT$' 3 'Agent LOG_FORMAT does not match API and Worker'
assert_count "$agent_render" '^[[:space:]]+- name: LOG_LEVEL$' 3 'Agent LOG_LEVEL does not match API and Worker'
assert_contains "$agent_render" '^[[:space:]]+- name: OTEL_SERVICE_VERSION$' 'Agent telemetry version is missing'
assert_contains "$agent_render" '^[[:space:]]+- name: AI_OBSERVABILITY_CAPTURE_DATABASE_SPANS$' 'Agent database-span diagnostic setting is missing'
assert_contains "$agent_render" '^[[:space:]]+value: "false"$' 'Agent database-span diagnostics are not disabled by default'
assert_count "$agent_render" '^[[:space:]]+checksum\.luna\.devops/managed-secrets:' 5 'Agent does not carry the managed Secret checksum'

agent_diagnostics_render="$tmp_dir/agent-diagnostics.yaml"
helm template luna-devops "$chart_dir" --namespace luna-devops \
  --set ai.enabled=true \
  --set ai.existingSecret=agent-auth \
  --set ai.agent.image.tag=agent-test-version \
  --set ai.agent.observabilityCaptureDatabaseSpans=true \
  --show-only templates/ai-agent.yaml > "$agent_diagnostics_render"
assert_contains "$agent_diagnostics_render" '^[[:space:]]+value: "agent-test-version"$' 'Agent image version was not reused for telemetry'
assert_contains "$agent_diagnostics_render" '^[[:space:]]+- name: AI_OBSERVABILITY_CAPTURE_DATABASE_SPANS$' 'Agent database-span diagnostic setting was not rendered'
assert_contains "$agent_diagnostics_render" '^[[:space:]]+value: "true"$' 'Agent database-span diagnostic setting was not configurable'

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
