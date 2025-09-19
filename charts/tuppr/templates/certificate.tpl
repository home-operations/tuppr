{{- if and .Values.webhook.enabled .Values.webhook.certManager.enabled }}
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: {{ include "tuppr.fullname" . }}-serving-cert
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "tuppr.labels" . | nindent 4 }}
spec:
  dnsNames:
    - {{ include "tuppr.webhookServiceName" . }}.{{ .Release.Namespace }}.svc
    - {{ include "tuppr.webhookServiceName" . }}.{{ .Release.Namespace }}.svc.cluster.local
  issuerRef:
    kind: Issuer
    name: {{ include "tuppr.fullname" . }}-selfsigned-issuer
  secretName: {{ include "tuppr.webhookCertName" . }}
  privateKey:
    rotationPolicy: {{ .Values.webhook.certManager.rotationPolicy | default "Always" }}
  duration: {{ .Values.webhook.certManager.duration | default "8760h" }}  # 1 year
  renewBefore: {{ .Values.webhook.certManager.renewBefore | default "720h" }}  # 30 days
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: {{ include "tuppr.fullname" . }}-selfsigned-issuer
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "tuppr.labels" . | nindent 4 }}
spec:
  selfSigned: {}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ include "tuppr.webhookServiceName" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "tuppr.labels" . | nindent 4 }}
spec:
  ports:
    - port: 443
      protocol: TCP
      targetPort: {{ .Values.webhook.port }}
      name: webhook
  selector:
    {{- include "tuppr.selectorLabels" . | nindent 4 }}
{{- end }}
