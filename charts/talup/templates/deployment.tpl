---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "talup.fullname" . }}
  labels:
    {{- include "talup.labels" . | nindent 4 }}
spec:
  {{- if not .Values.autoscaling.enabled }}
  replicas: {{ .Values.replicaCount }}
  {{- end }}
  selector:
    matchLabels:
      {{- include "talup.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      {{- with .Values.podAnnotations }}
      annotations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      labels:
        {{- include "talup.labels" . | nindent 8 }}
        {{- with .Values.podLabels }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
    spec:
      {{- with .Values.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      serviceAccountName: {{ include "talup.serviceAccountName" . }}
      securityContext:
        {{- toYaml .Values.podSecurityContext | nindent 8 }}
      containers:
        - name: manager
          securityContext:
            {{- toYaml .Values.securityContext | nindent 12 }}
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          command:
            - /manager
          args:
            - --leader-elect={{ .Values.controller.leaderElection.enabled }}
            {{- if .Values.controller.leaderElection.enabled }}
            - --leader-elect-lease-duration={{ .Values.controller.leaderElection.leaseDuration }}
            - --leader-elect-renew-deadline={{ .Values.controller.leaderElection.renewDeadline }}
            - --leader-elect-retry-period={{ .Values.controller.leaderElection.retryPeriod }}
            {{- end }}
            - --metrics-bind-address=:{{ .Values.controller.metrics.port }}
            - --health-probe-bind-address=:{{ .Values.controller.health.port }}
            - --talosctl-image={{ .Values.talup.talosctl.image.repository }}:{{ .Values.talup.talosctl.image.tag }}
            - --talos-config-secret={{ .Values.talup.talosctl.configSecret }}
          ports:
            - name: webhook-server
              containerPort: {{ .Values.webhook.port }}
              protocol: TCP
            {{- if .Values.controller.metrics.enabled }}
            - name: metrics
              containerPort: {{ .Values.controller.metrics.port }}
              protocol: TCP
            {{- end }}
            - name: health
              containerPort: {{ .Values.controller.health.port }}
              protocol: TCP
          livenessProbe:
            {{- toYaml .Values.livenessProbe | nindent 12 }}
          readinessProbe:
            {{- toYaml .Values.readinessProbe | nindent 12 }}
          resources:
            {{- toYaml .Values.resources | nindent 12 }}
          volumeMounts:
            - mountPath: /tmp/k8s-webhook-server/serving-certs
              name: cert
              readOnly: true
            {{- with .Values.volumeMounts }}
            {{- toYaml . | nindent 12 }}
            {{- end }}
      volumes:
        - name: cert
          secret:
            defaultMode: 420
            secretName: {{ include "talup.fullname" . }}-webhook-server-cert
        {{- with .Values.volumes }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
      {{- with .Values.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
