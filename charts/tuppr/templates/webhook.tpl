{{- if .Values.webhook.enabled }}
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: {{ include "tuppr.webhookConfigName" . }}
  labels:
    {{- include "tuppr.labels" . | nindent 4 }}
webhooks:
  - name: vkubernetesupgrade.kb.io
    admissionReviewVersions:
      - v1
    clientConfig:
      service:
        name: {{ include "tuppr.webhookServiceName" . }}
        namespace: {{ .Release.Namespace }}
        path: /validate-tuppr-home-operations-com-v1alpha1-kubernetesupgrade
    failurePolicy: Fail
    sideEffects: None
    rules:
      - apiGroups:
          - tuppr.home-operations.com
        apiVersions:
          - v1alpha1
        operations:
          - CREATE
          - UPDATE
        resources:
          - kubernetesupgrades
  - name: vtalosupgrade.kb.io
    admissionReviewVersions:
      - v1
    clientConfig:
      service:
        name: {{ include "tuppr.webhookServiceName" . }}
        namespace: {{ .Release.Namespace }}
        path: /validate-tuppr-home-operations-com-v1alpha1-talosupgrade
    failurePolicy: Fail
    sideEffects: None
    rules:
      - apiGroups:
          - tuppr.home-operations.com
        apiVersions:
          - v1alpha1
        operations:
          - CREATE
          - UPDATE
        resources:
          - talosupgrades
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
