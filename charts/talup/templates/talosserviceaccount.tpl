{{- if .Values.talos.serviceAccount.create }}
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: {{ include "talup.fullname" . }}-talos
  labels:
    {{- include "talup.labels" . | nindent 4 }}
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
    name: {{ include "talup.serviceAccountName" . }}
    namespace: {{ .Release.Namespace }}
---
apiVersion: talos.dev/v1alpha1
kind: ServiceAccount
metadata:
  name: {{ include "talup.talosServiceAccountName" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "talup.labels" . | nindent 4 }}
  {{- with .Values.talos.serviceAccount.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  roles:
    {{- toYaml .Values.talos.serviceAccount.roles | nindent 4 }}
{{- end }}
