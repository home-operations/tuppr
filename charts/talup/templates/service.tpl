apiVersion: v1
kind: Service
metadata:
  name: {{ include "talup.fullname" . }}-metrics-service
  labels:
    {{- include "talup.labels" . | nindent 4 }}
spec:
  type: {{ .Values.service.type }}
  ports:
    - port: {{ .Values.service.metricsPort }}
      targetPort: metrics
      protocol: TCP
      name: metrics
  selector:
    {{- include "talup.selectorLabels" . | nindent 4 }}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ include "talup.fullname" . }}-webhook-service
  labels:
    {{- include "talup.labels" . | nindent 4 }}
spec:
  ports:
    - port: 443
      protocol: TCP
      targetPort: {{ .Values.webhook.port }}
  selector:
    {{- include "talup.selectorLabels" . | nindent 4 }}
