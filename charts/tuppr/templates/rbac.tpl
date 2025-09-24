{{- if .Values.rbac.create -}}
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {{ include "tuppr.fullname" . }}-manager-role
  labels:
    {{- include "tuppr.labels" . | nindent 4 }}
  {{- with .Values.rbac.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
rules:
- apiGroups:
  - ""
  resources:
  - events
  verbs:
  - create
  - patch
- apiGroups:
  - ""
  resources:
  - nodes
  - pods
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
  - tuppr.home-operations.com
  resources:
  - kubernetesupgrades
  - talosupgrades
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - tuppr.home-operations.com
  resources:
  - kubernetesupgrades/finalizers
  - talosupgrades/finalizers
  verbs:
  - update
- apiGroups:
  - tuppr.home-operations.com
  resources:
  - kubernetesupgrades/status
  - talosupgrades/status
  verbs:
  - get
  - patch
  - update
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: {{ include "tuppr.fullname" . }}-manager-rolebinding
  labels:
    {{- include "tuppr.labels" . | nindent 4 }}
  {{- with .Values.rbac.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: {{ include "tuppr.fullname" . }}-manager-role
subjects:
- kind: ServiceAccount
  name: {{ include "tuppr.serviceAccountName" . }}
  namespace: {{ .Release.Namespace }}
---
{{- if .Values.controller.leaderElection.enabled }}
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: {{ include "tuppr.fullname" . }}-leader-election-role
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "tuppr.labels" . | nindent 4 }}
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
  name: {{ include "tuppr.fullname" . }}-leader-election-rolebinding
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "tuppr.labels" . | nindent 4 }}
  {{- with .Values.rbac.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: {{ include "tuppr.fullname" . }}-leader-election-role
subjects:
- kind: ServiceAccount
  name: {{ include "tuppr.serviceAccountName" . }}
  namespace: {{ .Release.Namespace }}
{{- end }}
{{- end }}
