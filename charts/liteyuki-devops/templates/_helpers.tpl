{{/*
Expand the chart name.
*/}}
{{- define "liteyuki-devops.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "liteyuki-devops.fullname" -}}
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

{{- define "liteyuki-devops.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "liteyuki-devops.selectorLabels" -}}
app.kubernetes.io/name: {{ include "liteyuki-devops.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "liteyuki-devops.labels" -}}
helm.sh/chart: {{ include "liteyuki-devops.chart" . }}
{{ include "liteyuki-devops.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "liteyuki-devops.componentLabels" -}}
{{ include "liteyuki-devops.labels" .root }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "liteyuki-devops.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "liteyuki-devops.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "liteyuki-devops.appSecretName" -}}
{{- default (printf "%s-app" (include "liteyuki-devops.fullname" .)) .Values.app.existingSecret -}}
{{- end -}}

{{- define "liteyuki-devops.connectionSecretName" -}}
{{- printf "%s-connection" (include "liteyuki-devops.fullname" .) -}}
{{- end -}}

{{- define "liteyuki-devops.postgresqlSecretName" -}}
{{- default (printf "%s-postgresql" (include "liteyuki-devops.fullname" .)) .Values.postgresql.auth.existingSecret -}}
{{- end -}}

{{- define "liteyuki-devops.postgresqlName" -}}
{{- printf "%s-postgresql" (include "liteyuki-devops.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "liteyuki-devops.redisName" -}}
{{- printf "%s-redis" (include "liteyuki-devops.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "liteyuki-devops.apiName" -}}
{{- printf "%s-api" (include "liteyuki-devops.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "liteyuki-devops.workerName" -}}
{{- printf "%s-worker" (include "liteyuki-devops.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "liteyuki-devops.apiMetricsName" -}}
{{- printf "%s-api-metrics" (include "liteyuki-devops.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "liteyuki-devops.workerMetricsName" -}}
{{- printf "%s-worker-metrics" (include "liteyuki-devops.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "liteyuki-devops.imageTag" -}}
{{- default .Chart.AppVersion .tag -}}
{{- end -}}

{{- define "liteyuki-devops.databaseUrlSecretName" -}}
{{- if .Values.externalDatabase.existingSecret -}}
{{- .Values.externalDatabase.existingSecret -}}
{{- else -}}
{{- include "liteyuki-devops.connectionSecretName" . -}}
{{- end -}}
{{- end -}}

{{- define "liteyuki-devops.databaseUrlSecretKey" -}}
{{- if .Values.externalDatabase.existingSecret -}}
{{- .Values.externalDatabase.urlKey -}}
{{- else -}}
database-url
{{- end -}}
{{- end -}}

{{- define "liteyuki-devops.redisAddrSecretName" -}}
{{- if .Values.externalRedis.existingSecret -}}
{{- .Values.externalRedis.existingSecret -}}
{{- else -}}
{{- include "liteyuki-devops.connectionSecretName" . -}}
{{- end -}}
{{- end -}}

{{- define "liteyuki-devops.redisAddrSecretKey" -}}
{{- if .Values.externalRedis.existingSecret -}}
{{- .Values.externalRedis.addrKey -}}
{{- else -}}
redis-addr
{{- end -}}
{{- end -}}

{{- define "liteyuki-devops.commonEnv" -}}
- name: APP_ENV
  value: {{ .Values.app.env | quote }}
- name: LOG_LEVEL
  value: {{ .Values.app.logLevel | quote }}
- name: SECRET_ENCRYPTION_KEY
  valueFrom:
    secretKeyRef:
      name: {{ include "liteyuki-devops.appSecretName" . }}
      key: {{ .Values.app.secretEncryptionKeyKey }}
- name: DATABASE_URL
  valueFrom:
    secretKeyRef:
      name: {{ include "liteyuki-devops.databaseUrlSecretName" . }}
      key: {{ include "liteyuki-devops.databaseUrlSecretKey" . }}
- name: REDIS_ADDR
  valueFrom:
    secretKeyRef:
      name: {{ include "liteyuki-devops.redisAddrSecretName" . }}
      key: {{ include "liteyuki-devops.redisAddrSecretKey" . }}
- name: METRICS_ENABLED
  value: {{ .Values.metrics.enabled | quote }}
- name: METRICS_PATH
  value: {{ .Values.metrics.path | quote }}
{{- range $name, $value := .Values.app.extraEnv }}
- name: {{ $name }}
  value: {{ $value | quote }}
{{- end }}
{{- end -}}
