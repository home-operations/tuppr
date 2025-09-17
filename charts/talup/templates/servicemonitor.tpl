{{- if and .Values.monitoring.serviceMonitor.enabled .Values.controller.metrics.enabled }}
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: {{ include "talup.fullname" . }}
  labels:
    {{- include "talup.labels" . | nindent 4 }}
    {{- with .Values.monitoring.serviceMonitor.labels }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
  {{- with .Values.monitoring.serviceMonitor.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  selector:
    matchLabels:
      {{- include "talup.selectorLabels" . | nindent 6 }}
  endpoints:
  - port: metrics
    interval: {{ .Values.monitoring.serviceMonitor.interval }}
    scrapeTimeout: {{ .Values.monitoring.serviceMonitor.scrapeTimeout }}
    path: /metrics
{{- end }}
