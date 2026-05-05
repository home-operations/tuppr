{{- if and .Values.monitoring.dashboards.enabled .Values.monitoring.dashboards.grafanaOperator.enabled (.Capabilities.APIVersions.Has "grafana.integreatly.org/v1beta1/GrafanaDashboard") }}
apiVersion: grafana.integreatly.org/v1beta1
kind: GrafanaDashboard
metadata:
  name: {{ include "tuppr.fullname" . }}-dashboard
  namespace: {{ .Values.monitoring.dashboards.namespace | default .Release.Namespace }}
  labels:
    {{- include "tuppr.labels" . | nindent 4 }}
spec:
  allowCrossNamespaceImport: {{ .Values.monitoring.dashboards.grafanaOperator.allowCrossNamespaceImport }}
  resyncPeriod: {{ .Values.monitoring.dashboards.grafanaOperator.resyncPeriod | default "10m" | quote }}
  {{- with .Values.monitoring.dashboards.grafanaOperator.folder }}
  folder: {{ . | quote }}
  {{- end }}
  instanceSelector:
    matchLabels:
    {{- if .Values.monitoring.dashboards.grafanaOperator.matchLabels }}
      {{- toYaml .Values.monitoring.dashboards.grafanaOperator.matchLabels | nindent 6 }}
    {{- else }}
      {{- fail "monitoring.dashboards.grafanaOperator.matchLabels must be set when grafanaOperator is enabled" }}
    {{- end }}
  configMapRef:
    name: {{ include "tuppr.fullname" . }}-dashboard
    key: tuppr.json
{{- end }}
