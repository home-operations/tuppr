---
apiVersion: v1
kind: Secret
metadata:
  labels:
    {{- include "tuppr.labels" . | nindent 4 }}
  {{- with .Values.controller.metrics.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  name: {{ include "tuppr.webhookCertName" . }}
  namespace: '{{ .Release.Namespace }}'
