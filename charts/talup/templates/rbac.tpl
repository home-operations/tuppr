{{- if .Values.rbac.create -}}
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {{ include "talup.fullname" . }}-manager-role
  labels:
    {{- include "talup.labels" . | nindent 4 }}
  {{- with .Values.rbac.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
rules:
- apiGroups:
  - ""
  resources:
  - nodes
  - secrets
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - batch
  resources:
  - jobs
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - talup.home-operations.com
  resources:
  - talosplans
  - kubernetesplans
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - talup.home-operations.com
  resources:
  - talosplans/finalizers
  - kubernetesplans/finalizers
  verbs:
  - update
- apiGroups:
  - talup.home-operations.com
  resources:
  - talosplans/status
  - kubernetesplans/status
  verbs:
  - get
  - patch
  - update
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: {{ include "talup.fullname" . }}-manager-rolebinding
  labels:
    {{- include "talup.labels" . | nindent 4 }}
  {{- with .Values.rbac.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: {{ include "talup.fullname" . }}-manager-role
subjects:
- kind: ServiceAccount
  name: {{ include "talup.serviceAccountName" . }}
  namespace: {{ .Release.Namespace }}
---
{{- if .Values.controller.leaderElection.enabled }}
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: {{ include "talup.fullname" . }}-leader-election-role
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "talup.labels" . | nindent 4 }}
  {{- with .Values.rbac.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
rules:
- apiGroups:
  - ""
  resources:
  - configmaps
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - coordination.k8s.io
  resources:
  - leases
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - ""
  resources:
  - events
  verbs:
  - create
  - patch
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: {{ include "talup.fullname" . }}-leader-election-rolebinding
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "talup.labels" . | nindent 4 }}
  {{- with .Values.rbac.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: {{ include "talup.fullname" . }}-leader-election-role
subjects:
- kind: ServiceAccount
  name: {{ include "talup.serviceAccountName" . }}
  namespace: {{ .Release.Namespace }}
{{- end }}
{{- end }}
