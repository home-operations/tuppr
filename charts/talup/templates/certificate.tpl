{{- if and .Values.webhook.enabled .Values.webhook.certManager.enabled }}
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: {{ include "talup.fullname" . }}-serving-cert
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "talup.labels" . | nindent 4 }}
spec:
  dnsNames:
    - {{ include "talup.webhookServiceName" . }}.{{ .Release.Namespace }}.svc
    - {{ include "talup.webhookServiceName" . }}.{{ .Release.Namespace }}.svc.cluster.local
  issuerRef:
    kind: Issuer
    name: {{ include "talup.fullname" . }}-selfsigned-issuer
  secretName: {{ include "talup.webhookCertName" . }}
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: {{ include "talup.fullname" . }}-selfsigned-issuer
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "talup.labels" . | nindent 4 }}
spec:
  selfSigned: {}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ include "talup.webhookServiceName" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "talup.labels" . | nindent 4 }}
spec:
  ports:
    - port: 443
      protocol: TCP
      targetPort: {{ .Values.webhook.port }}
      name: webhook
  selector:
    {{- include "talup.selectorLabels" . | nindent 4 }}
{{- end }}
