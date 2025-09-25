{{/*
Expand the name of the chart.
*/}}
{{- define "tuppr.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "tuppr.fullname" -}}
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
{{- define "tuppr.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "tuppr.labels" -}}
helm.sh/chart: {{ include "tuppr.chart" . }}
{{ include "tuppr.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "tuppr.selectorLabels" -}}
app.kubernetes.io/name: {{ include "tuppr.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "tuppr.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "tuppr.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the image name
*/}}
{{- define "tuppr.image" -}}
{{- printf "%s:%s" .Values.image.repository (.Values.image.tag | default .Chart.AppVersion) }}
{{- end }}

{{/*
Return the proper image pull policy
*/}}
{{- define "tuppr.imagePullPolicy" -}}
{{- .Values.image.pullPolicy | default "IfNotPresent" }}
{{- end }}

{{/*
Create webhook service name
*/}}
{{- define "tuppr.webhookServiceName" -}}
{{- printf "%s-webhook-service" (include "tuppr.fullname" .) }}
{{- end }}

{{/*
Create webhook certificate name
*/}}
{{- define "tuppr.webhookCertName" -}}
{{- printf "%s-webhook-server-cert" (include "tuppr.fullname" .) }}
{{- end }}

{{/*
Create metrics service name
*/}}
{{- define "tuppr.metricsServiceName" -}}
{{- printf "%s-metrics-service" (include "tuppr.fullname" .) }}
{{- end }}
