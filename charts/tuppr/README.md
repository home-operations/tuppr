# tuppr

![Version: 0.0.0](https://img.shields.io/badge/Version-0.0.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.0.0](https://img.shields.io/badge/AppVersion-0.0.0-informational?style=flat-square)

A Helm chart for tuppr - Talos Linux Upgrade Controller

**Homepage:** <https://github.com/home-operations/tuppr>

## Usage

tuppr ships as an OCI Helm chart (CRDs included). Install it, then create the
upgrade-plan custom resources it reconciles:

```sh
helm install tuppr oci://ghcr.io/home-operations/charts/tuppr \
  --namespace tuppr --create-namespace
```

The controller runs a single active replica (leader election), validates the
custom resources through an admission webhook (`webhook.enabled`), and exposes
Prometheus metrics (`controller.metrics`). See [`values.yaml`](values.yaml) for the
full set of controller, webhook, RBAC, and monitoring options.

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| home-operations | <contact@home-operations.com> |  |

## Source Code

* <https://github.com/home-operations/tuppr>

## Requirements

Kubernetes: `>=1.25.0-0`

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` | Affinity rules for pod scheduling. |
| controller.leaderElection.enabled | bool | `true` | Enable leader election (recommended; only one controller is active at a time). |
| controller.logLevel | string | `"debug"` | Controller log level: info or debug. |
| controller.metrics.annotations | object | `{}` | Annotations for the metrics Service. |
| controller.metrics.port | int | `8081` | Operational port: /metrics plus the /healthz and /readyz probes (plain HTTP; always on — restrict with a NetworkPolicy rather than disabling). |
| env | list | `[]` | Extra environment variables passed to the container. |
| fullnameOverride | string | `""` | Override the full release name. |
| image.digest | string | `""` | Pin the image by digest (sha256:…); when set, overrides the tag. |
| image.pullPolicy | string | `"IfNotPresent"` | Image pull policy. |
| image.repository | string | `"ghcr.io/home-operations/tuppr"` | Image repository. |
| image.tag | string | `""` | Overrides the image tag; defaults to the chart appVersion. |
| imagePullSecrets | list | `[]` | Image pull secrets for private registries. |
| livenessProbe | object | `{"httpGet":{"path":"/healthz","port":"metrics"},"initialDelaySeconds":15,"periodSeconds":20}` | Liveness probe. |
| monitoring.dashboards.annotations | object | `{}` | Annotations added to the dashboard ConfigMap. |
| monitoring.dashboards.enabled | bool | `false` | Render the Grafana dashboard ConfigMap (for grafana-operator or the kube-prometheus-stack sidecar). |
| monitoring.dashboards.grafanaOperator.allowCrossNamespaceImport | bool | `true` | If true allows for a Grafana in any namespace to access this GrafanaDashboard. |
| monitoring.dashboards.grafanaOperator.enabled | bool | `false` | Render a GrafanaDashboard CR (grafana-operator) instead of a sidecar ConfigMap. |
| monitoring.dashboards.grafanaOperator.folder | string | `""` | Folder to create the dashboard in. |
| monitoring.dashboards.grafanaOperator.matchLabels | object | `{}` | Selected labels for Grafana instance. |
| monitoring.dashboards.grafanaOperator.resyncPeriod | string | `"10m"` | Resync period for the Grafana operator to check for updates to the dashboard. |
| monitoring.dashboards.labels | object | `{}` | Labels added to the dashboard ConfigMap. |
| monitoring.dashboards.namespace | string | `""` | Namespace for the dashboard objects; defaults to the release namespace. |
| monitoring.prometheusRule.additionalRuleAnnotations | object | `{}` | Extra annotations added to every alert rule. |
| monitoring.prometheusRule.additionalRuleLabels | object | `{}` | Extra labels added to every alert rule. |
| monitoring.prometheusRule.annotations | object | `{}` | PrometheusRule annotations. |
| monitoring.prometheusRule.enabled | bool | `false` | Create a PrometheusRule with alerting rules. |
| monitoring.prometheusRule.labels | object | `{}` | PrometheusRule labels. |
| monitoring.serviceMonitor.annotations | object | `{}` | ServiceMonitor annotations. |
| monitoring.serviceMonitor.enabled | bool | `false` | Create a Prometheus Operator ServiceMonitor (requires its CRDs). |
| monitoring.serviceMonitor.interval | string | `"30s"` | Scrape interval. |
| monitoring.serviceMonitor.labels | object | `{}` | ServiceMonitor labels. |
| monitoring.serviceMonitor.metricRelabelings | list | `[]` | Prometheus metric relabelings. |
| monitoring.serviceMonitor.path | string | `"/metrics"` | Metrics path. |
| monitoring.serviceMonitor.podTargetLabels | list | `[]` | Pod target labels to copy from pods. |
| monitoring.serviceMonitor.relabelings | list | `[]` | Prometheus relabelings (applied before scraping). |
| monitoring.serviceMonitor.scrapeTimeout | string | `"10s"` | Scrape timeout. |
| monitoring.serviceMonitor.targetLabels | list | `[]` | Target labels to copy from the Service. |
| nameOverride | string | `""` | Override the chart name used in resource names. |
| nodeSelector | object | `{}` | Node selector for pod scheduling. |
| notification.enabled | bool | `false` | Enable upgrade notifications. |
| notification.secretKey | string | `"url"` | Key within the Secret holding the notification URL. |
| notification.secretName | string | `""` | Name of the Secret holding the notification (shoutrrr) URL. |
| podAnnotations | object | `{}` | Annotations added to the pod. |
| podLabels | object | `{}` | Labels added to the pod. |
| podSecurityContext | object | `{"fsGroup":65532,"runAsGroup":65532,"runAsNonRoot":true,"runAsUser":65532,"seccompProfile":{"type":"RuntimeDefault"}}` | Pod-level securityContext (runs as non-root uid/gid 65532). |
| priorityClassName | string | `"system-node-critical"` | Priority class name for pod scheduling. |
| rbac.annotations | object | `{}` | Annotations for the RBAC resources. |
| rbac.create | bool | `true` | Create the ClusterRole + ClusterRoleBinding the controller needs to manage nodes and jobs. |
| readinessProbe | object | `{"httpGet":{"path":"/readyz","port":"metrics"},"initialDelaySeconds":5,"periodSeconds":10}` | Readiness probe. |
| replicaCount | int | `1` | Number of controller replicas (only one is active at a time via leader election). |
| resources | object | `{}` | Pod resource requests/limits. |
| securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true,"runAsNonRoot":true,"runAsUser":65532}` | Container securityContext (no privilege escalation, read-only root filesystem, drops ALL capabilities). |
| service.port | int | `8080` | Service port for general access. |
| service.type | string | `"ClusterIP"` | Service type. |
| serviceAccount.annotations | object | `{}` | Annotations for the ServiceAccount (e.g. workload-identity bindings). |
| serviceAccount.automount | bool | `true` | Automount the API token (on by default: the controller talks to the cluster API). |
| serviceAccount.create | bool | `true` | Create a ServiceAccount. |
| serviceAccount.name | string | `""` | ServiceAccount name; generated from the release name if empty. |
| talosServiceAccount.create | bool | `true` | Create the talos.dev/v1alpha1 ServiceAccount that makes Talos generate the `<name>-talosconfig` Secret the controller mounts. Requires the Talos API-access CRD (a real Talos cluster). Set false on non-Talos clusters (e.g. e2e/kind) and provide that Secret yourself. |
| tolerations | list | `[{"key":"CriticalAddonsOnly","operator":"Exists"},{"effect":"NoSchedule","key":"node-role.kubernetes.io/control-plane","operator":"Exists"},{"effect":"NoSchedule","key":"node.kubernetes.io/unschedulable","operator":"Exists"}]` | Tolerations for pod scheduling (defaults keep the controller schedulable on control-plane and cordoned nodes so it can uncordon after an upgrade reboot). |
| volumeMounts | list | `[]` | Additional volume mounts on the container. |
| volumes | list | `[]` | Additional volumes on the Deployment. |
| webhook.annotations | object | `{}` | Annotations for the webhook Service. |
| webhook.enabled | bool | `true` | Enable the admission webhooks for validation. |
| webhook.port | int | `9443` | Webhook server port. |

---

_This README is generated by [helm-docs](https://github.com/norwoodj/helm-docs) from `Chart.yaml` and `values.yaml`. Edit those (or `README.md.gotmpl`) and run `mise run helm-docs`._
