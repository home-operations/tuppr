{{- if and .Values.monitoring.dashboards.grafanaOperator.enabled (.Capabilities.APIVersions.Has "grafana.integreatly.org/v1beta1/GrafanaDashboard") }}
apiVersion: grafana.integreatly.org/v1beta1
kind: GrafanaDashboard
metadata:
  name: {{ include "tuppr.fullname" . }}-dashboard
  namespace: {{ .Values.metrics.dashboard.namespace | default .Release.Namespace }}
spec:
  {{- toYaml .Values.metrics.dashboard.grafanaDashboard | nindent 2 }}
  configMapRef:
    name: {{ include "tuppr.fullname" . }}-dashboard
    key: tuprr.json
{{- end }}
