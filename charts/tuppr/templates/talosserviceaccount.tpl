{{- if .Values.talosServiceAccount.create }}
apiVersion: talos.dev/v1alpha1
kind: ServiceAccount
metadata:
  name: {{ include "tuppr.talosServiceAccountName" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "tuppr.labels" . | nindent 4 }}
  {{- with .Values.talosServiceAccount.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  roles:
    {{- toYaml .Values.talosServiceAccount.roles | nindent 4 }}
{{- end }}
