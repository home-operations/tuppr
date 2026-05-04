{{- if .Values.monitoring.dashboards.enabled }}
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "tuppr.fullname" . }}-dashboard
  namespace: {{ .Values.monitoring.dashboards.namespace | default .Release.Namespace }}
  {{- with .Values.monitoring.dashboards.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  labels:
    {{- include "tuppr.labels" . | nindent 4 }}
    {{- if not .Values.monitoring.dashboards.grafanaOperator.enabled }}
    grafana_dashboard: "1"
    {{- end }}
    {{- with .Values.monitoring.dashboards.labels }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
data:
  tuppr.json: |
    {{ .Files.Get "dashboards/tuppr.json" | indent 4 }}
{{- end }}
