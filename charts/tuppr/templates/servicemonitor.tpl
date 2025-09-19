{{- if .Values.monitoring.serviceMonitor.enabled }}
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: {{ include "tuppr.fullname" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "tuppr.labels" . | nindent 4 }}
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
      {{- include "tuppr.selectorLabels" . | nindent 6 }}
  endpoints:
    - port: metrics
      interval: {{ .Values.monitoring.serviceMonitor.interval | default "30s" }}
      scrapeTimeout: {{ .Values.monitoring.serviceMonitor.scrapeTimeout | default "10s" }}
      path: {{ .Values.monitoring.serviceMonitor.path | default "/metrics" }}
      {{- with .Values.monitoring.serviceMonitor.metricRelabelings }}
      metricRelabelings:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.monitoring.serviceMonitor.relabelings }}
      relabelings:
        {{- toYaml . | nindent 8 }}
      {{- end }}
  {{- with .Values.monitoring.serviceMonitor.targetLabels }}
  targetLabels:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .Values.monitoring.serviceMonitor.podTargetLabels }}
  podTargetLabels:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
