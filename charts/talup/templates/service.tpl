{{- if .Values.controller.metrics.enabled }}
apiVersion: v1
kind: Service
metadata:
  name: {{ include "talup.metricsServiceName" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "talup.labels" . | nindent 4 }}
  {{- with .Values.controller.metrics.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  type: ClusterIP
  ports:
    - port: {{ .Values.controller.metrics.port }}
      targetPort: metrics
      protocol: TCP
      name: metrics
  selector:
    {{- include "talup.selectorLabels" . | nindent 4 }}
{{- end }}
{{- if .Values.webhook.enabled }}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ include "talup.webhookServiceName" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "talup.labels" . | nindent 4 }}
  {{- with .Values.webhook.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  type: ClusterIP
  ports:
    - port: 443
      protocol: TCP
      targetPort: {{ .Values.webhook.port }}
      name: webhook
  selector:
    {{- include "talup.selectorLabels" . | nindent 4 }}
{{- end }}
