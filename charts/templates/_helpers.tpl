{{/*
Expand the name of the chart.
*/}}
{{- define "hyperfleet-adapter.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "hyperfleet-adapter.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "hyperfleet-adapter.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "hyperfleet-adapter.labels" -}}
helm.sh/chart: {{ include "hyperfleet-adapter.chart" . }}
{{ include "hyperfleet-adapter.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: hyperfleet
{{- end }}

{{/*
Selector labels
*/}}
{{- define "hyperfleet-adapter.selectorLabels" -}}
app.kubernetes.io/name: {{ include "hyperfleet-adapter.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Adapter-specific labels
*/}}
{{- define "hyperfleet-adapter.adapterLabels" -}}
{{ include "hyperfleet-adapter.labels" . }}
hyperfleet.io/adapter-type: {{ .adapterType }}
{{- end }}

{{/*
Adapter-specific selector labels
*/}}
{{- define "hyperfleet-adapter.adapterSelectorLabels" -}}
{{ include "hyperfleet-adapter.selectorLabels" . }}
hyperfleet.io/adapter-type: {{ .adapterType }}
{{- end }}

{{/*
Create the name of the service account to use for an adapter
*/}}
{{- define "hyperfleet-adapter.serviceAccountName" -}}
{{- if .Values.rbac.create }}
{{- printf "%s-%s" (include "hyperfleet-adapter.fullname" .) .adapterType }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Full image name
*/}}
{{- define "hyperfleet-adapter.image" -}}
{{- printf "%s/%s:%s" .Values.global.imageRegistry .Values.image.repository .Values.image.tag }}
{{- end }}

{{/*
Environment ConfigMap name
*/}}
{{- define "hyperfleet-adapter.environmentConfigMapName" -}}
{{- printf "%s-environment" (include "hyperfleet-adapter.fullname" .) }}
{{- end }}

{{/*
Broker ConfigMap name
*/}}
{{- define "hyperfleet-adapter.brokerConfigMapName" -}}
{{- printf "%s-broker" (include "hyperfleet-adapter.fullname" .) }}
{{- end }}

{{/*
Adapter ConfigMap name
*/}}
{{- define "hyperfleet-adapter.adapterConfigMapName" -}}
{{- printf "%s-%s-config" (include "hyperfleet-adapter.fullname" .) .adapterType }}
{{- end }}

{{/*
Generate broker environment variables
*/}}
{{- define "hyperfleet-adapter.brokerEnvVars" -}}
- name: BROKER_TYPE
  value: {{ .Values.broker.type | quote }}
- name: BROKER_MAX_CONCURRENCY
  value: {{ .Values.broker.maxConcurrency | quote }}
{{- if eq .Values.broker.type "pubsub" }}
- name: BROKER_PROJECT_ID
  value: {{ .Values.broker.pubsub.projectId | quote }}
- name: BROKER_SUBSCRIPTION_ID
  value: {{ .Values.broker.pubsub.subscriptionId | quote }}
{{- else if eq .Values.broker.type "awsSqs" }}
- name: BROKER_REGION
  value: {{ .Values.broker.awsSqs.region | quote }}
- name: BROKER_QUEUE_URL
  value: {{ .Values.broker.awsSqs.queueUrl | quote }}
{{- else if eq .Values.broker.type "rabbitmq" }}
- name: BROKER_HOST
  value: {{ .Values.broker.rabbitmq.host | quote }}
- name: BROKER_PORT
  value: {{ .Values.broker.rabbitmq.port | quote }}
- name: BROKER_QUEUE_NAME
  value: {{ .Values.broker.rabbitmq.queueName | quote }}
- name: BROKER_EXCHANGE
  value: {{ .Values.broker.rabbitmq.exchange | quote }}
- name: BROKER_USERNAME
  value: {{ .Values.broker.rabbitmq.username | quote }}
- name: BROKER_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ .Values.broker.rabbitmq.passwordSecretName }}
      key: {{ .Values.broker.rabbitmq.passwordSecretKey }}
{{- end }}
{{- end }}
