# tuppr - Talos Linux Upgrade Controller

A Kubernetes controller for managing automated upgrades of Talos Linux and Kubernetes.

## ✨ Features

### Core Capabilities

- 🚀 **Automated Talos node upgrades** with intelligent orchestration
- 🎯 **Kubernetes upgrades** - upgrade Kubernetes to newer versions
- 🔒 **Safe upgrade execution** - upgrades always run from healthy nodes (never self-upgrade)
- 📊 **Built-in health checks** - CEL-based expressions for custom cluster validation
- 🔄 **Configurable reboot modes** - default or powercycle options
- 📋 **Comprehensive status tracking** with real-time progress reporting
- ⚡ **Resilient job execution** with automatic retry and pod replacement
- 📈 **Prometheus metrics** - detailed monitoring of upgrade progress and health
- 🎯 **Per-node overrides** - use annotations to specify unique versions or schematics for specific nodes.

## 🚀 Quick Start

### Prerequisites

1. **Talos cluster** with API access configured
2. **Namespace** for the controller (e.g., `system-upgrade`)

### Installation

Allow Talos API access to the desired namespace by applying this config to all of you nodes:

```yaml
machine:
  features:
    kubernetesTalosAPIAccess:
      allowedKubernetesNamespaces:
        - system-upgrade # or the namespace the controller will be installed to
      allowedRoles:
        - os:admin
      enabled: true
```

Install the Helm chart:

```bash
# Install via Helm
helm install tuppr oci://ghcr.io/home-operations/charts/tuppr \
  --version 0.1.0 \
  --namespace system-upgrade
```

### Basic Usage

#### Talos Node Upgrades

Create a `TalosUpgrade` resource:

```yaml
apiVersion: tuppr.home-operations.com/v1alpha1
kind: TalosUpgrade
metadata:
  name: cluster
spec:
  talos:
    # renovate: datasource=docker depName=ghcr.io/siderolabs/installer
    version: v1.11.0  # Required - target Talos version

  policy:
    debug: true          # Optional, verbose logging
    force: false         # Optional, skip etcd health checks
    rebootMode: default  # Optional, default|powercycle
    placement: soft      # Optional, hard|soft
    stage: false         # Optional, stage upgrade
    timeout: 30m         # Optional, per-node upgrade timeout

  # Custom health checks (optional)
  healthChecks:
    - apiVersion: v1
      kind: Node
      expr: status.conditions.exists(c, c.type == "Ready" && c.status == "True")

  # Talosctl configuration (optional)
  talosctl:
    image:
      repository: ghcr.io/siderolabs/talosctl  # Optional, default
      tag: v1.11.0                             # Optional, auto-detected
      pullPolicy: IfNotPresent                 # Optional, default

  # Maintenance windows (optional)
  maintenance:
    windows:
      - start: "0 2 * * 0"    # Cron expression (Sunday 02:00)
        duration: "4h"         # How long window stays open
        timezone: "UTC"        # IANA timezone, default UTC
```

#### Kubernetes Upgrades

Create a `KubernetesUpgrade` resource:

```yaml
apiVersion: tuppr.home-operations.com/v1alpha1
kind: KubernetesUpgrade
metadata:
  name: kubernetes
spec:
  kubernetes:
    # renovate: datasource=docker depName=ghcr.io/siderolabs/kubelet
    version: v1.34.0  # Required - target Kubernetes version

  # Custom health checks (optional)
  healthChecks:
    - apiVersion: v1
      kind: Node
      expr: status.conditions.exists(c, c.type == "Ready" && c.status == "True")
      timeout: 10m

  # Talosctl configuration (optional)
  talosctl:
    image:
      repository: ghcr.io/siderolabs/talosctl  # Optional, default
      tag: v1.11.0                             # Optional, auto-detected
      pullPolicy: IfNotPresent                 # Optional, default

  # Maintenance windows (optional)
  maintenance:
    windows:
      - start: "0 2 * * 0"    # Cron expression (Sunday 02:00)
        duration: "4h"         # How long window stays open
        timezone: "UTC"        # IANA timezone, default UTC
```

## 🎯 Advanced Configuration

### Health Checks

Define custom health checks using [CEL expressions](https://cel.dev/). These health checks are evaluated before each upgrade and run concurrently.

```yaml
healthChecks:
  # Check all nodes are ready
  - apiVersion: v1
    kind: Node
    expr: |
      status.conditions.filter(c, c.type == "Ready").all(c, c.status == "True")
    timeout: 10m

  # Check specific deployment replicas
  - apiVersion: apps/v1
    kind: Deployment
    name: critical-app
    namespace: production
    expr: status.readyReplicas == status.replicas

  # Check custom resources
  - apiVersion: ceph.rook.io/v1
    kind: CephCluster
    name: rook-ceph
    namespace: rook-ceph
    expr: status.ceph.health in ["HEALTH_OK"]
```

### Upgrade Policies (TalosUpgrade only)

Fine-tune upgrade behavior:

```yaml
policy:
  # Enable debug logging for troubleshooting
  debug: true

  # Force upgrade even if etcd is unhealthy (dangerous!)
  force: true

  # Controls how strictly upgrade jobs avoid the target node
  placement: hard  # or "soft"

  # Use powercycle reboot for problematic nodes
  rebootMode: powercycle  # or "default"

  # Stage upgrade then reboot to apply (2 total reboots)
  stage: false
```

### Maintenance Windows

Control when upgrades start using cron-based maintenance windows. Running upgrades always complete without interruption.

```yaml
maintenance:
  windows:
    - start: "0 2 * * 0"      # Sunday 02:00
      duration: "4h"           # Max 168h, warn if <1h
      timezone: "Europe/Paris" # IANA timezone, default UTC
```

- Upgrades only start during open windows (stays `Pending` otherwise)
- Multiple windows create union (any open window allows start)
- In-progress upgrades always complete (never interrupted)
- TalosUpgrade re-checks between nodes
- Empty config: upgrades start immediately (backwards compatible)

### Per-Node Overrides

Tuppr supports overriding the global TalosUpgrade configuration on a per-node basis using Kubernetes annotations. This is useful for testing new versions on a canary node or handling nodes with different hardware schematics.

| Annotation | Description | Example |
| -------- | ------- | ------- |
| tuppr.home-operations.com/version | Overrides the target Talos version for this node. | v1.12.1 |
| tuppr.home-operations.com/schematic | Overrides the Talos schematic hash for this node. | b55fbf... |


Example: Applying an override

```Bash
# Upgrade a specific node to a different version than the global policy
kubectl annotate node worker-01 tuppr.home-operations.com/version="v1.12.1"

# Apply a custom schematic (with specific extensions) to one node
kubectl annotate node worker-02 tuppr.home-operations.com/schematic="314b18a3f89d..."
```

How it works:
- The controller checks if a node version or schematic matches the annotation instead of the global TalosUpgrade spec.
- If an inconsistency is found, an upgrade job is triggered for that node using the override values.

## 📊 Monitoring & Metrics

### Prometheus Metrics

Tuppr exposes comprehensive Prometheus metrics for monitoring upgrade progress, health check performance, and job execution:

#### Talos Upgrade Metrics

```prometheus
# Current phase of Talos upgrades (0=Pending, 1=InProgress, 2=Completed, 3=Failed)
tuppr_talos_upgrade_phase{name="cluster", phase="InProgress"} 1

# Node counts for Talos upgrades
tuppr_talos_upgrade_nodes_total{name="cluster"} 5
tuppr_talos_upgrade_nodes_completed{name="cluster"} 3
tuppr_talos_upgrade_nodes_failed{name="cluster"} 0

# Duration of Talos upgrade phases (histogram)
tuppr_talos_upgrade_duration_seconds{name="cluster", phase="InProgress"}
```

#### Kubernetes Upgrade Metrics

```prometheus
# Current phase of Kubernetes upgrades (0=Pending, 1=InProgress, 2=Completed, 3=Failed)
tuppr_kubernetes_upgrade_phase{name="kubernetes", phase="Completed"} 2

# Duration of Kubernetes upgrade phases (histogram)
tuppr_kubernetes_upgrade_duration_seconds{name="kubernetes", phase="InProgress"}
```

#### Health Check Metrics

```prometheus
# Time taken for health checks to pass (histogram)
tuppr_health_check_duration_seconds{upgrade_type="talos", upgrade_name="cluster"}

# Total number of health check failures (counter)
tuppr_health_check_failures_total{upgrade_type="talos", upgrade_name="cluster", check_index="0"}
```

#### Job Execution Metrics

```prometheus
# Number of active upgrade jobs
tuppr_upgrade_jobs_active{upgrade_type="talos"} 1

# Time taken for upgrade jobs to complete (histogram)
tuppr_upgrade_job_duration_seconds{upgrade_type="talos", node_name="worker-01", result="success"}
```

#### Maintenance Window Metrics

```prometheus
# Whether upgrade is currently inside a maintenance window (1=inside, 0=outside)
tuppr_maintenance_window_active{upgrade_type="talos", name="talos"} 0

# Unix timestamp of the next maintenance window start
tuppr_maintenance_window_next_open_timestamp{upgrade_type="talos", name="talos"} 1735603200
```

### Grafana Dashboard Examples

#### Upgrade Progress Panel

```promql
# Upgrade phase status
tuppr_talos_upgrade_phase or tuppr_kubernetes_upgrade_phase

# Node upgrade progress for Talos
tuppr_talos_upgrade_nodes_completed / tuppr_talos_upgrade_nodes_total * 100

# Health check success rate
rate(tuppr_health_check_failures_total[5m]) == 0
```

#### Performance Monitoring

```promql
# Average health check duration
histogram_quantile(0.95, rate(tuppr_health_check_duration_seconds_bucket[5m]))

# Job completion rate
rate(tuppr_upgrade_job_duration_seconds_count{result="success"}[5m])

# Active jobs by type
sum(tuppr_upgrade_jobs_active) by (upgrade_type)
```

### Alerting Rules

Example Prometheus alerting rules:

```yaml
groups:
  - name: tuppr
    rules:
      - alert: TupprUpgradeFailed
        expr: tuppr_talos_upgrade_phase{phase="Failed"} == 3 or tuppr_kubernetes_upgrade_phase{phase="Failed"} == 3
        for: 0m
        labels:
          severity: critical
        annotations:
          summary: "Tuppr upgrade failed"
          description: "Upgrade {{ $labels.name }} has failed"

      - alert: TupprUpgradeStuck
        expr: |
          (
            tuppr_talos_upgrade_phase{phase="InProgress"} == 1 and
            increase(tuppr_talos_upgrade_nodes_completed[30m]) == 0
          ) or (
            tuppr_kubernetes_upgrade_phase{phase="InProgress"} == 1
          )
        for: 30m
        labels:
          severity: warning
        annotations:
          summary: "Tuppr upgrade appears stuck"
          description: "Upgrade {{ $labels.name }} has been in progress for 30+ minutes without completing nodes"

      - alert: TupprHealthCheckFailures
        expr: rate(tuppr_health_check_failures_total[5m]) > 0.1
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High health check failure rate"
          description: "Health checks for {{ $labels.upgrade_name }} are failing frequently"
```

## 🔧 Operations

### Monitoring Upgrades

```bash
# Watch Talos upgrade progress
kubectl get talosupgrade -w

# Watch Kubernetes upgrade progress
kubectl get kubernetesupgrade -w

# Check detailed status
kubectl describe talosupgrade cluster-upgrade
kubectl describe kubernetesupgrade kubernetes

# View upgrade logs
kubectl logs -f deployment/tuppr -n system-upgrade

# Force a node to a specific version/schematic
kubectl annotate node <node-name> tuppr.home-operations.com/version="v1.10.7"
kubectl annotate node <node-name> tuppr.home-operations.com/schematic="<hash>"

# Check if a node has overrides applied
kubectl get nodes -o custom-columns=NAME:.metadata.name,VERSION-OVERRIDE:.metadata.annotations."tuppr\.home-operations\.com/version"

# Check metrics endpoint
kubectl port-forward -n system-upgrade deployment/tuppr 8080:8080
curl http://localhost:8080/metrics | grep tuppr_
```

### Suspending Upgrades

Suspending upgrades can be useful if you want to upgrade manually and not have the controller interfere.

```bash
# Suspend Talos upgrade
kubectl annotate talosupgrade cluster-upgrade tuppr.home-operations.com/suspend="true"

# Suspend Kubernetes upgrade
kubectl annotate kubernetesupgrade kubernetes tuppr.home-operations.com/suspend="true"

# Remove the suspend annotation to resume
kubectl annotate talosupgrade cluster-upgrade tuppr.home-operations.com/suspend-
kubectl annotate kubernetesupgrade kubernetes tuppr.home-operations.com/suspend-
```

### Troubleshooting

```bash
# Reset failed Talos upgrade
kubectl annotate talosupgrade talos tuppr.home-operations.com/reset="$(date)"

# Reset failed Kubernetes upgrade
kubectl annotate kubernetesupgrade kubernetes tuppr.home-operations.com/reset="$(date)"

# Check job logs
kubectl logs job/tuppr-xyz -n system-upgrade

# Check controller health
kubectl get pods -n system-upgrade -l app.kubernetes.io/name=tuppr

# View metrics for debugging
kubectl port-forward -n system-upgrade deployment/tuppr 8080:8080
curl http://localhost:8080/metrics | grep -E "(tuppr_.*_phase|tuppr_.*_duration)"
```

### Emergency Procedures

```bash
# Pause all upgrades (scale down controller)
kubectl scale deployment tuppr --replicas=0 -n system-upgrade

# Emergency cleanup
kubectl delete talosupgrade --all
kubectl delete kubernetesupgrade --all
kubectl delete jobs -l app.kubernetes.io/name=talos-upgrade -n system-upgrade
kubectl delete jobs -l app.kubernetes.io/name=kubernetes-upgrade -n system-upgrade

# Resume operations
kubectl scale deployment tuppr --replicas=1 -n system-upgrade
```

## 📋 Upgrade Comparison

| Feature | TalosUpgrade | KubernetesUpgrade |
|---------|--------------|-------------------|
| **Scope** | Talos nodes | Kubernetes cluster |
| **Multiple CRs** | ❌ Only one per cluster | ❌ Only one per cluster |
| **Execution** | Sequential node-by-node | Single controller node |
| **Reboot Required** | ✅ Yes | ❌ No |
| **Health Checks** | ✅ Before each node | ✅ Before upgrade |
| **Concurrent Execution** | ❌ Blocked by other upgrades | ❌ Blocked by other upgrades |
| **Handling Failures** | ❌ Manual | ❌ Manual |
| **Metrics** | ✅ Comprehensive | ✅ Comprehensive |

### Important Resource Constraints

- **TalosUpgrade**: Only **one** `TalosUpgrade` resource is allowed per cluster. This constraint simplifies the upgrade orchestration by processing all nodes sequentially in a single upgrade workflow, eliminating the need for complex queueing and coordination between multiple upgrade resources. The admission webhook will reject attempts to create additional `TalosUpgrade` resources.

- **KubernetesUpgrade**: Only **one** `KubernetesUpgrade` resource is allowed per cluster. This constraint exists because Kubernetes upgrades affect the entire cluster, and multiple concurrent upgrades would conflict with each other. The admission webhook will reject attempts to create additional `KubernetesUpgrade` resources.

- **Cross-Upgrade Coordination**: TalosUpgrade and KubernetesUpgrade resources **cannot run concurrently**. If one upgrade is in progress (status.phase == "InProgress"), the other will wait in a "Pending" state until the active upgrade completes. This prevents conflicts between Talos node changes and Kubernetes cluster changes that could destabilize the cluster.

### Upgrade Coordination Examples

```yaml
# ✅ Valid: Single TalosUpgrade resource
apiVersion: tuppr.home-operations.com/v1alpha1
kind: TalosUpgrade
metadata:
  name: talos
spec:
  talos:
    version: v1.11.0
---
# ❌ Invalid: Second TalosUpgrade will be rejected by webhook
apiVersion: tuppr.home-operations.com/v1alpha1
kind: TalosUpgrade
metadata:
  name: another-talos  # This will fail validation
spec:
  talos:
    version: v1.11.1
---
# ✅ Valid: Single KubernetesUpgrade resource
apiVersion: tuppr.home-operations.com/v1alpha1
kind: KubernetesUpgrade
metadata:
  name: kubernetes
spec:
  kubernetes:
    version: v1.34.0
---
# ❌ Invalid: Second KubernetesUpgrade will be rejected by webhook
apiVersion: tuppr.home-operations.com/v1alpha1
kind: KubernetesUpgrade
metadata:
  name: another-kubernetes  # This will fail validation
spec:
  kubernetes:
    version: v1.35.0
```

### Cross-Upgrade Coordination Behavior

**Scenario 1: TalosUpgrade starts first**

```bash
kubectl apply -f talos-upgrade.yaml
# ✅ TalosUpgrade starts immediately (phase: InProgress)

kubectl apply -f kubernetes-upgrade.yaml
# ⏳ KubernetesUpgrade waits (phase: Pending)
#    message: "Waiting for Talos upgrade 'talos' to complete before starting Kubernetes upgrade"

# After TalosUpgrade completes (phase: Completed)
# ✅ KubernetesUpgrade starts automatically (phase: InProgress)
```

**Scenario 2: KubernetesUpgrade starts first**

```bash
kubectl apply -f kubernetes-upgrade.yaml
# ✅ KubernetesUpgrade starts immediately (phase: InProgress)

kubectl apply -f talos-upgrade.yaml
# ⏳ TalosUpgrade waits (phase: Pending)
#    message: "Waiting for Kubernetes upgrade 'kubernetes' to complete before starting Talos upgrade"

# After KubernetesUpgrade completes (phase: Completed)
# ✅ TalosUpgrade starts automatically (phase: InProgress)
```

**Scenario 3: Only one upgrade type needed**

```bash
# If you only need Talos upgrades
kubectl apply -f talos-upgrade.yaml
# ✅ Starts immediately - no blocking

# If you only need Kubernetes upgrades
kubectl apply -f kubernetes-upgrade.yaml
# ✅ Starts immediately - no blocking
```

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

## 📄 License

This project is licensed under the **GNU Affero General Public License v3.0** - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- **[Talos Linux](https://www.talos.dev/)** - The modern OS for Kubernetes that inspired this project
- **[System Upgrade Controller](https://github.com/rancher/system-upgrade-controller)** - Inspiration for upgrade orchestration patterns
- **[Kubebuilder](https://book.kubebuilder.io/)** - Excellent framework for building Kubernetes controllers
- **[Controller Runtime](https://github.com/kubernetes-sigs/controller-runtime)** - Powerful runtime for Kubernetes controllers
- **[CEL](https://cel.dev/)** - Common Expression Language for flexible health checks
- **[Prometheus](https://prometheus.io/)** - Monitoring and alerting toolkit for metrics collection

---

**⭐ If this project helps you, please consider giving it a star!**

For questions, issues, or feature requests, please visit our [GitHub Issues](https://github.com/home-operations/tuppr/issues).
