{{- if .Values.rbac.create -}}
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {{ include "talup.fullname" . }}-manager-role
  labels:
    {{- include "talup.labels" . | nindent 4 }}
rules:
- apiGroups:
  - ""
  resources:
  - nodes
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
  - upgrade.home-operations.com
  resources:
  - talosplans
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - upgrade.home-operations.com
  resources:
  - talosplans/finalizers
  verbs:
  - update
- apiGroups:
  - upgrade.home-operations.com
  resources:
  - talosplans/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - upgrade.home-operations.com
  resources:
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
  - upgrade.home-operations.com
  resources:
  - kubernetesplans/finalizers
  verbs:
  - update
- apiGroups:
  - upgrade.home-operations.com
  resources:
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
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: {{ include "talup.fullname" . }}-manager-role
subjects:
- kind: ServiceAccount
  name: {{ include "talup.serviceAccountName" . }}
  namespace: {{ .Release.Namespace }}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: {{ include "talup.fullname" . }}-leader-election-role
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "talup.labels" . | nindent 4 }}
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
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: {{ include "talup.fullname" . }}-leader-election-role
subjects:
- kind: ServiceAccount
  name: {{ include "talup.serviceAccountName" . }}
  namespace: {{ .Release.Namespace }}
{{- end }}
