{{- if .Values.controller.metrics.enabled }}
apiVersion: v1
kind: Service
metadata:
  name: {{ include "tuppr.metricsServiceName" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "tuppr.labels" . | nindent 4 }}
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
    {{- include "tuppr.selectorLabels" . | nindent 4 }}
{{- end }}
