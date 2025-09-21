---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "tuppr.fullname" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "tuppr.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      {{- include "tuppr.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      {{- with .Values.podAnnotations }}
      annotations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      labels:
        {{- include "tuppr.labels" . | nindent 8 }}
        {{- with .Values.podLabels }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
    spec:
      enableServiceLinks: false
      {{- with .Values.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      serviceAccountName: {{ include "tuppr.serviceAccountName" . }}
      priorityClassName: system-node-critical
      securityContext:
        {{- toYaml .Values.podSecurityContext | nindent 8 }}
      containers:
        - name: manager
          securityContext:
            {{- toYaml .Values.securityContext | nindent 12 }}
          image: {{ include "tuppr.image" . }}
          imagePullPolicy: {{ include "tuppr.imagePullPolicy" . }}
          command:
            - /manager
          args:
            - --leader-elect={{ .Values.controller.leaderElection.enabled }}
            - --metrics-bind-address=:{{ .Values.controller.metrics.port }}
            - --health-probe-bind-address=:{{ .Values.controller.health.port }}
            - --talosconfig-secret={{ include "tuppr.talosServiceAccountName" . }}
          env:
            - name: CONTROLLER_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
          ports:
            {{- if .Values.webhook.enabled }}
            - name: webhook-server
              containerPort: {{ .Values.webhook.port }}
              protocol: TCP
            {{- end }}
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
            {{- if .Values.webhook.enabled }}
            - mountPath: /tmp/k8s-webhook-server/serving-certs
              name: cert
              readOnly: true
            {{- end }}
            {{- with .Values.volumeMounts }}
            {{- toYaml . | nindent 12 }}
            {{- end }}
      volumes:
        {{- if .Values.webhook.enabled }}
        - name: cert
          secret:
            defaultMode: 420
            secretName: {{ include "tuppr.webhookCertName" . }}
        {{- end }}
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
