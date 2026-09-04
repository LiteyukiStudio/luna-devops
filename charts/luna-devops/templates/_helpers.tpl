{{/*
Expand the chart name.
*/}}
{{- define "luna-devops.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "luna-devops.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "luna-devops.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "luna-devops.selectorLabels" -}}
app.kubernetes.io/name: {{ include "luna-devops.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "luna-devops.labels" -}}
helm.sh/chart: {{ include "luna-devops.chart" . }}
{{ include "luna-devops.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "luna-devops.componentLabels" -}}
{{ include "luna-devops.labels" .root }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "luna-devops.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "luna-devops.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "luna-devops.appSecretName" -}}
{{- default (printf "%s-app" (include "luna-devops.fullname" .)) .Values.app.existingSecret -}}
{{- end -}}

{{- define "luna-devops.initialAdminSecretName" -}}
{{- default (printf "%s-initial-admin" (include "luna-devops.fullname" .)) .Values.api.initialAdmin.existingSecret -}}
{{- end -}}

{{- define "luna-devops.connectionSecretName" -}}
{{- printf "%s-connection" (include "luna-devops.fullname" .) -}}
{{- end -}}

{{- define "luna-devops.postgresqlSecretName" -}}
{{- default (printf "%s-postgresql" (include "luna-devops.fullname" .)) .Values.postgresql.auth.existingSecret -}}
{{- end -}}

{{- define "luna-devops.postgresqlName" -}}
{{- printf "%s-postgresql" (include "luna-devops.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "luna-devops.redisName" -}}
{{- printf "%s-redis" (include "luna-devops.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "luna-devops.apiName" -}}
{{- printf "%s-api" (include "luna-devops.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "luna-devops.workerName" -}}
{{- printf "%s-worker" (include "luna-devops.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "luna-devops.sharedConfigName" -}}
{{- printf "%s-config" (include "luna-devops.fullname" .) -}}
{{- end -}}

{{- define "luna-devops.apiConfigName" -}}
{{- printf "%s-config" (include "luna-devops.apiName" .) -}}
{{- end -}}

{{- define "luna-devops.workerConfigName" -}}
{{- printf "%s-config" (include "luna-devops.workerName" .) -}}
{{- end -}}

{{- define "luna-devops.agentName" -}}
{{- printf "%s-agent" (include "luna-devops.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "luna-devops.aiSecretName" -}}
{{- default (printf "%s-ai" (include "luna-devops.fullname" .)) .Values.ai.existingSecret -}}
{{- end -}}

{{- define "luna-devops.apiMetricsName" -}}
{{- printf "%s-api-metrics" (include "luna-devops.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "luna-devops.imageTag" -}}
{{- default .Chart.AppVersion .tag -}}
{{- end -}}

{{- define "luna-devops.volumeTransferJobImage" -}}
{{- if .Values.volumeTransfer.jobImage -}}
{{- .Values.volumeTransfer.jobImage -}}
{{- else -}}
{{- printf "%s:%s" .Values.worker.image.repository (include "luna-devops.imageTag" (dict "Chart" .Chart "tag" .Values.worker.image.tag)) -}}
{{- end -}}
{{- end -}}

{{- define "luna-devops.databaseUrlSecretName" -}}
{{- if .Values.externalDatabase.existingSecret -}}
{{- .Values.externalDatabase.existingSecret -}}
{{- else -}}
{{- include "luna-devops.connectionSecretName" . -}}
{{- end -}}
{{- end -}}

{{- define "luna-devops.databaseUrlSecretKey" -}}
{{- if .Values.externalDatabase.existingSecret -}}
{{- .Values.externalDatabase.urlKey -}}
{{- else -}}
database-url
{{- end -}}
{{- end -}}

{{- define "luna-devops.redisURLSecretName" -}}
{{- if and .Values.redis.enabled .Values.redis.auth.existingSecret -}}
{{- .Values.redis.auth.existingSecret -}}
{{- else if .Values.redis.enabled -}}
{{- include "luna-devops.connectionSecretName" . -}}
{{- else if .Values.externalRedis.existingSecret -}}
{{- .Values.externalRedis.existingSecret -}}
{{- else -}}
{{- include "luna-devops.connectionSecretName" . -}}
{{- end -}}
{{- end -}}

{{- define "luna-devops.redisURLSecretKey" -}}
{{- if .Values.redis.enabled -}}
{{- .Values.redis.auth.urlKey -}}
{{- else if .Values.externalRedis.existingSecret -}}
{{- .Values.externalRedis.urlKey -}}
{{- else -}}
redis-url
{{- end -}}
{{- end -}}

{{- define "luna-devops.commonEnv" -}}
- name: APP_ENV
  value: {{ .Values.app.env | quote }}
- name: LOG_FORMAT
  value: {{ .Values.app.logFormat | quote }}
- name: LOG_COLOR
  value: {{ .Values.app.logColor | quote }}
- name: LOG_LEVEL
  value: {{ .Values.app.logLevel | quote }}
- name: SECRET_ENCRYPTION_KEY
  valueFrom:
    secretKeyRef:
      name: {{ include "luna-devops.appSecretName" . }}
      key: {{ .Values.app.secretEncryptionKeyKey }}
- name: DATABASE_URL
  valueFrom:
    secretKeyRef:
      name: {{ include "luna-devops.databaseUrlSecretName" . }}
      key: {{ include "luna-devops.databaseUrlSecretKey" . }}
- name: REDIS_ADDR
  valueFrom:
    secretKeyRef:
      name: {{ include "luna-devops.redisURLSecretName" . }}
      key: {{ include "luna-devops.redisURLSecretKey" . }}
{{- if .Values.observability.otlpEndpoint }}
- name: OTEL_EXPORTER_OTLP_ENDPOINT
  value: {{ .Values.observability.otlpEndpoint | quote }}
- name: OTEL_RESOURCE_ATTRIBUTES
  value: {{ .Values.observability.resourceAttributes | quote }}
{{- if .Values.observability.existingSecret }}
- name: OTEL_EXPORTER_OTLP_HEADERS
  valueFrom:
    secretKeyRef:
      name: {{ .Values.observability.existingSecret }}
      key: {{ .Values.observability.headersKey }}
{{- end }}
{{- end }}
{{- end -}}

{{- define "luna-devops.extraEnv" -}}
{{- $component := .component -}}
{{- $managed := list
  "APP_ENV"
  "LOG_FORMAT"
  "LOG_COLOR"
  "LOG_LEVEL"
  "SECRET_ENCRYPTION_KEY"
  "DATABASE_URL"
  "REDIS_ADDR"
  "REDIS_PASSWORD"
  "POSTGRES_PASSWORD"
  "OTEL_EXPORTER_OTLP_ENDPOINT"
  "OTEL_RESOURCE_ATTRIBUTES"
  "OTEL_EXPORTER_OTLP_HEADERS"
  "OTEL_EXPORTER_OTLP_TRACES_HEADERS"
  "PUBLIC_BASE_URL"
  "VOLUME_TRANSFER_MAX_BYTES"
  "VOLUME_TRANSFER_JOB_IMAGE"
  "AI_INTERNAL_SECRET"
  "INITIAL_ADMIN_EMAIL"
  "INITIAL_ADMIN_NAME"
  "INITIAL_ADMIN_PASSWORD"
  "INITIAL_ADMIN_LANGUAGE"
  "API_ADDR"
  "APP_CORS_ORIGINS"
  "TRUSTED_PROXY_CIDRS"
  "METRICS_ENABLED"
  "METRICS_ADDR"
  "METRICS_PATH"
  "AI_ASSISTANT_AVAILABLE"
  "AI_AGENT_BASE_URL"
  "AI_AGENT_TIMEOUT"
  "API_DB_MAX_OPEN_CONNS"
  "API_DB_MAX_IDLE_CONNS"
  "API_DB_CONN_MAX_LIFETIME"
  "API_DB_CONN_MAX_IDLE_TIME"
  "WORKER_DB_MAX_OPEN_CONNS"
  "WORKER_DB_MAX_IDLE_CONNS"
  "WORKER_DB_CONN_MAX_LIFETIME"
  "WORKER_DB_CONN_MAX_IDLE_TIME"
  "DEPLOY_ROLLOUT_TIMEOUT_SECONDS"
  "CERT_MANAGER_CLUSTER_ISSUER"
  "BUILD_EXECUTOR_IMAGE"
  "BUILD_EGRESS_MODE"
  "BUILD_JOB_TIMEOUT_SECONDS"
  "BUILD_JOB_TTL_SECONDS"
  "BUILD_CACHE_ENABLED"
  "BUILD_CACHE_TAG"
  "BUILD_PRIVATE_EGRESS_CIDRS"
  "BUILD_PRIVATE_EGRESS_PORTS"
  "BUILD_BLOCKED_EGRESS_CIDRS"
-}}
{{- range $name, $value := .values }}
{{- if has $name $managed }}
{{- fail (printf "%s is managed by the chart and cannot be set through %s.extraEnv" $name $component) }}
{{- end }}
- name: {{ $name | quote }}
  value: {{ $value | quote }}
{{- end }}
{{- end -}}
