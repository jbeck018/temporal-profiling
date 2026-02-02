{{/*
Expand the name of the chart.
*/}}
{{- define "temporal-profiler.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "temporal-profiler.fullname" -}}
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
{{- define "temporal-profiler.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "temporal-profiler.labels" -}}
helm.sh/chart: {{ include "temporal-profiler.chart" . }}
{{ include "temporal-profiler.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "temporal-profiler.selectorLabels" -}}
app.kubernetes.io/name: {{ include "temporal-profiler.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "temporal-profiler.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "temporal-profiler.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Get the secret name for credentials
*/}}
{{- define "temporal-profiler.secretName" -}}
{{- if .Values.existingSecrets.credentials }}
{{- .Values.existingSecrets.credentials }}
{{- else }}
{{- include "temporal-profiler.fullname" . }}-credentials
{{- end }}
{{- end }}

{{/*
Get the secret name for TLS
*/}}
{{- define "temporal-profiler.tlsSecretName" -}}
{{- if .Values.existingSecrets.tls }}
{{- .Values.existingSecrets.tls }}
{{- else }}
{{- include "temporal-profiler.fullname" . }}-tls
{{- end }}
{{- end }}
