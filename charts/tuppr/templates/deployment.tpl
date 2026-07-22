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
      {{- with .Values.priorityClassName }}
      priorityClassName: {{ . }}
      {{- end }}
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
            - --log-level={{ .Values.controller.logLevel }}
            - --leader-elect={{ .Values.controller.leaderElection.enabled }}
            - --metrics-bind-address=:{{ .Values.controller.metrics.port }}
            - --talosconfig-secret={{ include "tuppr.serviceAccountName" . }}-talosconfig
            - --metrics-service-name={{ include "tuppr.metricsServiceName" . }}
            {{- if .Values.webhook.enabled }}
            - --webhook-config-name={{ include "tuppr.webhookConfigName" . }}
            - --webhook-service-name={{ include "tuppr.webhookServiceName" . }}
            - --webhook-secret-name={{ include "tuppr.webhookCertName" . }}
            {{- end }}
          env:
            - name: CONTROLLER_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
            - name: CONTROLLER_NODE_NAME
              valueFrom:
                fieldRef:
                  fieldPath: spec.nodeName
            - name: CONTROLLER_POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            {{- if and .Values.notification.enabled .Values.notification.secretName }}
            - name: NOTIFICATION_URL
              valueFrom:
                secretKeyRef:
                  name: {{ .Values.notification.secretName }}
                  key: {{ .Values.notification.secretKey }}
            {{- end }}
            {{- if .Values.notification.enabled }}
            {{- with .Values.notification.titleTemplate }}
            - name: NOTIFICATION_TITLE_TEMPLATE
              value: {{ . | quote }}
            {{- end }}
            {{- with .Values.notification.messageTemplate }}
            - name: NOTIFICATION_MESSAGE_TEMPLATE
              value: {{ . | quote }}
            {{- end }}
            {{- end }}
            {{- with .Values.env }}
            {{- toYaml . | nindent 12 }}
            {{- end }}
          ports:
            {{- if .Values.webhook.enabled }}
            - name: webhook-server
              containerPort: {{ .Values.webhook.port }}
              protocol: TCP
            {{- end }}
            # Serves /metrics plus the /healthz and /readyz probes (single
            # operational port; see cmd/main.go).
            - name: metrics
              containerPort: {{ .Values.controller.metrics.port }}
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
            {{- end }}
            - name: talosconfig
              mountPath: /var/run/secrets/talos.dev
              readOnly: true
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
        - name: talosconfig
          secret:
            secretName: {{ include "tuppr.serviceAccountName" . }}-talosconfig
            defaultMode: 420
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
