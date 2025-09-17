{{- if .Values.webhook.enabled }}
{{- if .Values.webhook.certManager.enabled }}
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: {{ include "talup.fullname" . }}-serving-cert
  labels:
    {{- include "talup.labels" . | nindent 4 }}
spec:
  dnsNames:
  - {{ include "talup.fullname" . }}-webhook-service.{{ .Release.Namespace }}.svc
  - {{ include "talup.fullname" . }}-webhook-service.{{ .Release.Namespace }}.svc.cluster.local
  issuerRef:
    kind: Issuer
    name: {{ include "talup.fullname" . }}-selfsigned-issuer
  secretName: {{ include "talup.fullname" . }}-webhook-server-cert
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: {{ include "talup.fullname" . }}-selfsigned-issuer
  labels:
    {{- include "talup.labels" . | nindent 4 }}
spec:
  selfSigned: {}
{{- end }}
{{- end }}
