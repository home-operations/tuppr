{{/*
Expand the name of the chart.
*/}}
{{- define "talup.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "talup.fullname" -}}
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
{{- define "talup.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "talup.labels" -}}
helm.sh/chart: {{ include "talup.chart" . }}
{{ include "talup.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "talup.selectorLabels" -}}
app.kubernetes.io/name: {{ include "talup.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "talup.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "talup.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the talos service account to use (also used as secret name)
*/}}
{{- define "talup.talosServiceAccountName" -}}
{{- if and .Values.talos.serviceAccount .Values.talos.serviceAccount.name }}
{{- .Values.talos.serviceAccount.name }}
{{- else }}
{{- include "talup.fullname" . }}
{{- end }}
{{- end }}

{{/*
Create the image name
*/}}
{{- define "talup.image" -}}
{{- printf "%s:%s" .Values.image.repository (.Values.image.tag | default .Chart.AppVersion) }}
{{- end }}

{{/*
Return the proper image pull policy
*/}}
{{- define "talup.imagePullPolicy" -}}
{{- .Values.image.pullPolicy | default "IfNotPresent" }}
{{- end }}

{{/*
Create webhook service name
*/}}
{{- define "talup.webhookServiceName" -}}
{{- printf "%s-webhook-service" (include "talup.fullname" .) }}
{{- end }}

{{/*
Create webhook certificate name
*/}}
{{- define "talup.webhookCertName" -}}
{{- printf "%s-webhook-server-cert" (include "talup.fullname" .) }}
{{- end }}

{{/*
Create metrics service name
*/}}
{{- define "talup.metricsServiceName" -}}
{{- printf "%s-metrics-service" (include "talup.fullname" .) }}
{{- end }}
