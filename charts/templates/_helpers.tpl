{{/*
Expand the name of the chart.
*/}}
{{- define "hyperfleet-adapter.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
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
app.kubernetes.io/version: {{ .Values.image.tag | default .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.labels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "hyperfleet-adapter.selectorLabels" -}}
app.kubernetes.io/name: {{ include "hyperfleet-adapter.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "hyperfleet-adapter.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "hyperfleet-adapter.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the adapter ConfigMap to use
*/}}
{{- define "hyperfleet-adapter.adapterConfigMapName" -}}
{{- if .Values.adapterConfig.configMapName }}
{{- .Values.adapterConfig.configMapName }}
{{- else }}
{{- printf "%s-config" (include "hyperfleet-adapter.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Create the name of the adapter task ConfigMap to use
*/}}
{{- define "hyperfleet-adapter.adapterTaskConfigMapName" -}}
{{- if .Values.adapterTaskConfig.configMapName }}
{{- .Values.adapterTaskConfig.configMapName }}
{{- else }}
{{- printf "%s-task" (include "hyperfleet-adapter.fullname" .) }}
{{- end }}
{{- end }}

{{- define "hyperfleet-adapter.helmValidateAdapterIsConfigured" -}}
{{- if .Values.adapterConfig.create -}}
  {{- $hasYaml := not (empty .Values.adapterConfig.yaml) -}}
  {{- $hasFiles := not (empty .Values.adapterConfig.files) -}}
  {{- if or $hasYaml $hasFiles -}}
true
  {{- else -}}
{{- fail "When .Values.adapterConfig.create is true, either .Values.adapterConfig.yaml or .Values.adapterConfig.files must be provided." -}}
  {{- end -}}
{{- else if (hasKey .Values.adapterConfig "configMapName") -}}
  {{- if and .Values.adapterConfig.configMapName (ne .Values.adapterConfig.configMapName "") -}}
true
  {{- else -}}
{{- fail "If .Values.adapterConfig.configMapName is specified, it must have a non-blank value." -}}
  {{- end -}}
{{- else -}}
{{- fail "Either .Values.adapterConfig.create must be true or a non-blank .Values.adapterConfig.configMapName must be provided." -}}
{{- end -}}
{{- end }}

{{- define "hyperfleet-adapter.helmValidateAdapterTaskIsConfigured" -}}
{{- if .Values.adapterTaskConfig.create -}}
  {{- $hasYaml := not (empty .Values.adapterTaskConfig.yaml) -}}
  {{- $hasFiles := not (empty .Values.adapterTaskConfig.files) -}}
  {{- $hasExternal := not (empty .Values.adapterTaskConfig.external) -}}
  {{- if and $hasYaml (or $hasFiles $hasExternal) -}}
{{- fail "Cannot set .Values.adapterTaskConfig.yaml with .Values.adapterTaskConfig.files or .Values.adapterTaskConfig.external - these modes are mutually exclusive." -}}
  {{- end -}}
  {{- if or $hasYaml $hasFiles $hasExternal -}}
true
  {{- else -}}
{{- fail "When .Values.adapterTaskConfig.create is true, at least one of .Values.adapterTaskConfig.yaml, .Values.adapterTaskConfig.files, or .Values.adapterTaskConfig.external must be provided." -}}
  {{- end -}}
{{- else if (hasKey .Values.adapterTaskConfig "configMapName") -}}
  {{- if and .Values.adapterTaskConfig.configMapName (ne .Values.adapterTaskConfig.configMapName "") -}}
true
  {{- else -}}
{{- fail "If .Values.adapterTaskConfig.configMapName is specified, it must have a non-blank value." -}}
  {{- end -}}
{{- else -}}
{{- fail "Either .Values.adapterTaskConfig.create must be true or a non-blank .Values.adapterTaskConfig.configMapName must be provided." -}}
{{- end -}}
{{- end }}

{{/*
Render a probe with defaults for httpGet.
*/}}
{{- define "hyperfleet-adapter.renderProbe" -}}
{{- $probe := .probe -}}
{{- $defaultPath := .defaultPath -}}
{{- $defaultPort := .defaultPort | default "http" -}}
{{- if $probe.httpGet }}
httpGet:
  path: {{ $probe.httpGet.path | default $defaultPath }}
  port: {{ $probe.httpGet.port | default $defaultPort }}
  {{- if $probe.httpGet.scheme }}
  scheme: {{ $probe.httpGet.scheme }}
  {{- end }}
{{- else if $probe.tcpSocket }}
tcpSocket:
  port: {{ $probe.tcpSocket.port }}
{{- else if $probe.exec }}
exec:
  command:
    {{- toYaml $probe.exec.command | nindent 4 }}
{{- else }}
httpGet:
  path: {{ $defaultPath }}
  port: {{ $defaultPort }}
{{- end }}
{{- with $probe.initialDelaySeconds }}
initialDelaySeconds: {{ . }}
{{- end }}
{{- with $probe.periodSeconds }}
periodSeconds: {{ . }}
{{- end }}
{{- with $probe.timeoutSeconds }}
timeoutSeconds: {{ . }}
{{- end }}
{{- with $probe.failureThreshold }}
failureThreshold: {{ . }}
{{- end }}
{{- with $probe.successThreshold }}
successThreshold: {{ . }}
{{- end }}
{{- end }}

{{/*
Get the apiGroup for a Kubernetes resource.
Maps resource names to their correct API group for RBAC rules.
Returns empty string for core API resources, otherwise the appropriate apiGroup.
*/}}
{{- define "hyperfleet-adapter.apiGroup" -}}
{{- $resource := . -}}
{{- $appsResources := list "deployments" "statefulsets" "daemonsets" "replicasets" -}}
{{- $batchResources := list "jobs" "cronjobs" "jobs/status" -}}
{{- $rbacResources := list "roles" "rolebindings" "clusterroles" "clusterrolebindings" -}}
{{- if has $resource $appsResources -}}
apps
{{- else if has $resource $batchResources -}}
batch
{{- else if has $resource $rbacResources -}}
rbac.authorization.k8s.io
{{- end -}}
{{- end }}

{{/*
Validate required values that must not remain as placeholders.
*/}}
{{- define "hyperfleet-adapter.validateValues" -}}
{{- $registry := trim (toString .Values.image.registry) -}}
{{- if or (not $registry) (eq $registry "CHANGE_ME") -}}
{{- fail "image.registry must be set (e.g. --set image.registry=quay.io)" -}}
{{- end -}}
{{- $repository := trim (toString .Values.image.repository) -}}
{{- if or (not $repository) (eq $repository "CHANGE_ME") -}}
{{- fail "image.repository must be set (e.g. --set image.repository=openshift-hyperfleet/hyperfleet-adapter)" -}}
{{- end -}}
{{- if not (trim (toString .Values.image.tag)) -}}
{{- fail "image.tag must be set (e.g. --set image.tag=abc1234)" -}}
{{- end -}}
{{- if .Values.adapterConfig.hyperfleetApi.auth.enabled -}}
{{- if not (hasPrefix "/" .Values.adapterConfig.hyperfleetApi.auth.tokenPath) -}}
{{- fail "adapterConfig.hyperfleetApi.auth.tokenPath must be an absolute path (start with /)" -}}
{{- end -}}
{{- if eq (.Values.adapterConfig.hyperfleetApi.auth.tokenPath | base) "" -}}
{{- fail "adapterConfig.hyperfleetApi.auth.tokenPath must include a filename, not just a directory" -}}
{{- end -}}
{{- end -}}
{{- end }}

{{/*
Validate deprecated snake_case keys have not been used.
*/}}
{{- define "hyperfleet-adapter.validateDeprecatedKeys" -}}
{{- if .Values.adapterConfig }}
{{- if hasKey .Values.adapterConfig "hyperfleet_api" }}
{{- fail "adapterConfig.hyperfleet_api has been renamed to adapterConfig.hyperfleetApi (camelCase). Please update your values." }}
{{- end -}}
{{- if .Values.adapterConfig.hyperfleetApi }}
{{- if hasKey .Values.adapterConfig.hyperfleetApi "base_url" }}
{{- fail "adapterConfig.hyperfleetApi.base_url has been renamed to adapterConfig.hyperfleetApi.baseUrl (camelCase). Please update your values." }}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end }}

{{/*
Name for cluster-scoped resources (ClusterRole, ClusterRoleBinding).
Appends the release namespace to the fullname so that two installations in
different namespaces never collide on the same cluster-scoped resource name.
*/}}
{{- define "hyperfleet-adapter.clusterScopedName" -}}
{{- printf "%s-%s" (include "hyperfleet-adapter.fullname" .) .Release.Namespace | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Validate no key collisions between adapterTaskConfig.external and adapterTaskConfig.files
Also validate that all file paths in adapterTaskConfig.files actually exist
*/}}
{{- define "hyperfleet-adapter.validateTaskConfigKeys" -}}
{{- if and .Values.adapterTaskConfig.external .Values.adapterTaskConfig.files }}
  {{- range $name, $value := .Values.adapterTaskConfig.external }}
    {{- $externalKey := printf "%s.yaml" $name }}
    {{- if hasKey $.Values.adapterTaskConfig.files $externalKey }}
      {{- fail (printf "ConfigMap key collision: '%s' exists in both adapterTaskConfig.external and adapterTaskConfig.files" $externalKey) }}
    {{- end }}
  {{- end }}
{{- end }}
{{- if .Values.adapterTaskConfig.files }}
  {{- range $key, $path := .Values.adapterTaskConfig.files }}
    {{- $content := $.Files.Get $path }}
    {{- if not $content }}
      {{- fail (printf "adapterTaskConfig.files.%s: file not found or empty at path '%s'" $key $path) }}
    {{- end }}
  {{- end }}
{{- end }}
{{- end }}