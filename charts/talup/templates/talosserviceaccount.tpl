{{- if .Values.talos.serviceAccount.create }}
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: {{ include "tuppr.fullname" . }}-talos
  labels:
    {{- include "tuppr.labels" . | nindent 4 }}
  {{- with .Values.talos.serviceAccount.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
  - kind: ServiceAccount
    name: {{ include "tuppr.serviceAccountName" . }}
    namespace: {{ .Release.Namespace }}
---
apiVersion: talos.dev/v1alpha1
kind: ServiceAccount
metadata:
  name: {{ include "tuppr.talosServiceAccountName" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "tuppr.labels" . | nindent 4 }}
  {{- with .Values.talos.serviceAccount.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  roles:
    {{- toYaml .Values.talos.serviceAccount.roles | nindent 4 }}
{{- end }}
