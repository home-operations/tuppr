{{- if and .Values.monitoring.prometheusRule.enabled .Capabilities.APIVersions.Has "monitoring.coreos.com/v1" }}
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: {{ include "tuppr.fullname" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "tuppr.labels" . | nindent 4 }}
    {{- with .Values.monitoring.prometheusRule.labels }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
  {{- with .Values.monitoring.prometheusRule.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  groups:
    - name: tuppr.talosupgrade
      rules:
        # Fires as soon as at least one node has permanently failed. The upgrade
        # has stopped and will not proceed without manual intervention.
        - alert: TalosUpgradeNodeFailed
          expr: tuppr_talos_upgrade_nodes_failed > 0
          for: 1m
          labels:
            severity: critical
          annotations:
            summary: >-
              Talos upgrade {{ "{{" }} $labels.name {{ "}}" }} has
              {{ "{{" }} $value {{ "}}" }} failed node(s)
            description: >-
              Talos upgrade {{ "{{" }} $labels.name {{ "}}" }} has
              {{ "{{" }} $value {{ "}}" }} node(s) that failed to upgrade.
              The upgrade has stopped and requires manual intervention
              (e.g. remove the failed nodes from status or fix the underlying
              issue and reset with the reset annotation).

        # Fires when the upgrade object itself is in Failed phase.
        - alert: TalosUpgradeFailed
          expr: tuppr_talos_upgrade_phase{phase="Failed"} > 0
          for: 1m
          labels:
            severity: critical
          annotations:
            summary: >-
              Talos upgrade {{ "{{" }} $labels.name {{ "}}" }} is in Failed phase
            description: >-
              Talos upgrade {{ "{{" }} $labels.name {{ "}}" }} has entered the
              Failed phase. Check controller logs and node conditions for details.

        # Fires when an upgrade stays in an active phase longer than expected.
        # Typical node upgrade (talosctl + reboot) takes 5–15 min; 1 h means
        # something is genuinely stuck (crashed pod, unresponsive node, etc.).
        - alert: TalosUpgradeStuck
          expr: >-
            tuppr_talos_upgrade_phase{phase=~"Upgrading|Rebooting|Draining|HealthChecking"} > 0
          for: {{ .Values.monitoring.prometheusRule.stuckDuration | default "1h" }}
          labels:
            severity: warning
          annotations:
            summary: >-
              Talos upgrade {{ "{{" }} $labels.name {{ "}}" }} stuck in
              {{ "{{" }} $labels.phase {{ "}}" }} phase
            description: >-
              Talos upgrade {{ "{{" }} $labels.name {{ "}}" }} has been in the
              {{ "{{" }} $labels.phase {{ "}}" }} phase for more than
              {{ .Values.monitoring.prometheusRule.stuckDuration | default "1h" }}.
              Check the upgrade job and target node for errors.

    - name: tuppr.kubernetesupgrade
      rules:
        # Fires when the Kubernetes upgrade object itself is in Failed phase.
        - alert: KubernetesUpgradeFailed
          expr: tuppr_kubernetes_upgrade_phase{phase="Failed"} > 0
          for: 1m
          labels:
            severity: critical
          annotations:
            summary: >-
              Kubernetes upgrade {{ "{{" }} $labels.name {{ "}}" }} is in Failed phase
            description: >-
              Kubernetes upgrade {{ "{{" }} $labels.name {{ "}}" }} has entered the
              Failed phase. Check controller logs and the upgrade job for details.

        # Fires when the Kubernetes upgrade stays in an active phase too long.
        # A full control-plane upgrade typically completes in under 30 min.
        - alert: KubernetesUpgradeStuck
          expr: >-
            tuppr_kubernetes_upgrade_phase{phase=~"Upgrading|HealthChecking"} > 0
          for: 45m
          labels:
            severity: warning
          annotations:
            summary: >-
              Kubernetes upgrade {{ "{{" }} $labels.name {{ "}}" }} stuck in
              {{ "{{" }} $labels.phase {{ "}}" }} phase
            description: >-
              Kubernetes upgrade {{ "{{" }} $labels.name {{ "}}" }} has been in the
              {{ "{{" }} $labels.phase {{ "}}" }} phase for more than 45 minutes.
              Check the upgrade job and control-plane nodes for errors.

    - name: tuppr.jobs
      rules:
        # Fires when an upgrade job has been running longer than the configured
        # active deadline (talos: ~100 min worst-case, kubernetes: 60 min).
        # 1 h is a safe threshold that catches truly stuck jobs without false
        # positives on slow-but-healthy upgrades.
        - alert: UpgradeJobRunningTooLong
          expr: tuppr_upgrade_jobs_active > 0
          for: 1h
          labels:
            severity: warning
          annotations:
            summary: >-
              {{ "{{" }} $labels.upgrade_type | title {{ "}}" }} upgrade job running
              for over 1 hour
            description: >-
              A {{ "{{" }} $labels.upgrade_type {{ "}}" }} upgrade job has been
              active for more than 1 hour, which exceeds the expected duration.
              The job may be stuck. Check the job logs in the controller namespace.
{{- end }}
