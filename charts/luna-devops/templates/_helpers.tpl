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

{{- define "luna-devops.apiMetricsName" -}}
{{- printf "%s-api-metrics" (include "luna-devops.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "luna-devops.workerMetricsName" -}}
{{- printf "%s-worker-metrics" (include "luna-devops.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "luna-devops.imageTag" -}}
{{- default .Chart.AppVersion .tag -}}
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

{{- define "luna-devops.redisAddrSecretName" -}}
{{- if .Values.redis.enabled -}}
{{- include "luna-devops.connectionSecretName" . -}}
{{- else if .Values.externalRedis.existingSecret -}}
{{- .Values.externalRedis.existingSecret -}}
{{- else -}}
{{- include "luna-devops.connectionSecretName" . -}}
{{- end -}}
{{- end -}}

{{- define "luna-devops.redisAddrSecretKey" -}}
{{- if .Values.redis.enabled -}}
redis-addr
{{- else if .Values.externalRedis.existingSecret -}}
{{- .Values.externalRedis.addrKey -}}
{{- else -}}
redis-addr
{{- end -}}
{{- end -}}

{{- define "luna-devops.redisCredentialSecretName" -}}
{{- if and .Values.redis.enabled .Values.redis.auth.existingSecret -}}
{{- .Values.redis.auth.existingSecret -}}
{{- else if and (not .Values.redis.enabled) .Values.externalRedis.existingSecret -}}
{{- .Values.externalRedis.existingSecret -}}
{{- else -}}
{{- include "luna-devops.connectionSecretName" . -}}
{{- end -}}
{{- end -}}

{{- define "luna-devops.redisUsernameSecretKey" -}}
{{- .Values.externalRedis.usernameKey -}}
{{- end -}}

{{- define "luna-devops.redisPasswordSecretKey" -}}
{{- if .Values.redis.enabled -}}
{{- .Values.redis.auth.passwordKey -}}
{{- else -}}
{{- .Values.externalRedis.passwordKey -}}
{{- end -}}
{{- end -}}

{{- define "luna-devops.redisDBSecretKey" -}}
{{- if .Values.redis.enabled -}}
redis-db
{{- else -}}
{{- .Values.externalRedis.dbKey -}}
{{- end -}}
{{- end -}}

{{- define "luna-devops.commonEnv" -}}
- name: APP_ENV
  value: {{ .Values.app.env | quote }}
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
      name: {{ include "luna-devops.redisAddrSecretName" . }}
      key: {{ include "luna-devops.redisAddrSecretKey" . }}
- name: REDIS_USERNAME
  {{- if .Values.redis.enabled }}
  value: "default"
  {{- else }}
  valueFrom:
    secretKeyRef:
      name: {{ include "luna-devops.redisCredentialSecretName" . }}
      key: {{ include "luna-devops.redisUsernameSecretKey" . }}
      optional: true
  {{- end }}
- name: REDIS_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ include "luna-devops.redisCredentialSecretName" . }}
      key: {{ include "luna-devops.redisPasswordSecretKey" . }}
- name: REDIS_DB
  {{- if .Values.redis.enabled }}
  value: "0"
  {{- else }}
  valueFrom:
    secretKeyRef:
      name: {{ include "luna-devops.redisCredentialSecretName" . }}
      key: {{ include "luna-devops.redisDBSecretKey" . }}
      optional: true
  {{- end }}
- name: METRICS_ENABLED
  value: {{ .Values.metrics.enabled | quote }}
- name: METRICS_PATH
  value: {{ .Values.metrics.path | quote }}
{{- range $name, $value := .Values.app.extraEnv }}
- name: {{ $name }}
  value: {{ $value | quote }}
{{- end }}
{{- end -}}
