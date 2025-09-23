# tuppr - Talos Linux Upgrade Controller

A Kubernetes controller for managing automated upgrades of Talos Linux and Kubernetes verisons.

## ✨ Features

### Core Capabilities

- 🚀 **Automated Talos node upgrades** with intelligent orchestration
- 🎯 **Kubernetes upgrades** - upgrade Kubernetes to newer versions *(coming soon)*
- 🔒 **Safe upgrade execution** - upgrades always run from healthy nodes (never self-upgrade)
- 📊 **Built-in health checks** - CEL-based expressions for custom cluster validation
- 🎛️ **Flexible node targeting** with advanced label selectors
- 🔄 **Configurable reboot modes** - default or powercycle options
- 📋 **Comprehensive status tracking** with real-time progress reporting
- ⚡ **Resilient job execution** with automatic retry and pod replacement

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
        - system-upgrade
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

Create a `TalosUpgrade` resource:

```yaml
apiVersion: tuppr.home-operations.com/v1alpha1
kind: TalosUpgrade
metadata:
  name: cluster-upgrade
spec:
  # renovate: datasource=docker depName=ghcr.io/siderolabs/installer
  version: v1.11.0  # Required - target Talos version

  upgradePolicy:
    debug: false           # Optional, verbose logging
    force: false           # Optional, skip etcd health checks
    rebootMode: default    # Optional, default|powercycle
    placementPreset: soft  # Optional, hard|soft

  # Target specific nodes (optional - defaults to all nodes)
  nodeLabelSelector:
    matchLabels:
      node-role.kubernetes.io/control-plane: ""
    matchExpressions:
      - key: kubernetes.io/hostname
        operator: NotIn
        values: ["maintenance-node"]

  # Custom health checks (optional)
  healthChecks:
    - apiVersion: v1
      kind: Node
      expr: status.conditions.exists(c, c.type == "Ready" && c.status == "True")

    - apiVersion: ceph.rook.io/v1
      kind: CephCluster
      name: rook-ceph
      namespace: rook-ceph
      expr: status.ceph.health in ["HEALTH_OK"]
      timeout: 10m  # Optional, default

  # Talosctl configuration (optional)
  talosctl:
    image:
      repository: ghcr.io/siderolabs/talosctl  # Optional, default
      tag: v1.11.0                             # Optional, auto-detected
      pullPolicy: IfNotPresent                 # Optional, default
```

## 🎯 Advanced Configuration

### Health Checks

Define custom health checks using [CEL expressions](https://cel.dev/). These health checks are evaluated before each node upgrade and ran concurrently.

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
  - apiVersion: argoproj.io/v1alpha1
    kind: Application
    namespace: argocd
    expr: |
      status.health.status == "Healthy" &&
      status.sync.status == "Synced"
```

### Node Targeting

Precise control over which nodes to upgrade:

```yaml
nodeLabelSelector:
  matchLabels:
    environment: production
    node-role.kubernetes.io/worker: ""

  matchExpressions:
    # Exclude specific nodes
    - key: kubernetes.io/hostname
      operator: NotIn
      values: ["node-1", "node-2"]

    # Target nodes with specific zones
    - key: topology.kubernetes.io/zone
      operator: In
      values: ["us-west-2a", "us-west-2b"]

    # Exclude maintenance windows
    - key: maintenance
      operator: DoesNotExist
```

### Upgrade Policies

Fine-tune upgrade behavior:

```yaml
upgradePolicy:
  # Enable debug logging for troubleshooting
  debug: true

  # Force upgrade even if etcd is unhealthy (dangerous!)
  force: false

  # Controls how strictly upgrade jobs avoid the target node
  placementPreset: hard  # or "soft"

  # Use powercycle reboot for problematic nodes
  rebootMode: powercycle  # or "default"
```

## 🔧 Operations

### Suspending Upgrades

Suspending upgrades can be useful if you want to upgrade Talos from the command-line and not have the controller interfere with your changes.

```bash
# Suspend with "true" value
kubectl annotate talosupgrade cluster-upgrade tuppr.home-operations.com/suspend="true"

# Remove the suspend annotation to resume
kubectl annotate talosupgrade cluster-upgrade tuppr.home-operations.com/suspend-
```

### Monitoring Upgrades

```bash
# Watch upgrade progress
kubectl get talosupgrade -w

# Check detailed status
kubectl describe talosupgrade cluster-upgrade

# View upgrade logs
kubectl logs -f deployment/tuppr -n system-upgrade
```

### Troubleshooting

```bash
# Reset failed upgrade
kubectl annotate talosupgrade cluster-upgrade tuppr.home-operations.com/reset="$(date)"

# Check job logs
kubectl logs job/tuppr-xyz -n system-upgrade

# Check controller health
kubectl get pods -n system-upgrade -l app.kubernetes.io/name=tuppr
```

### Emergency Procedures

```bash
# Pause all upgrades (scale down controller)
kubectl scale deployment tuppr --replicas=0 -n system-upgrade

# Emergency cleanup
kubectl delete talosupgrade --all
kubectl delete jobs -l app.kubernetes.io/name=talos-upgrade -n system-upgrade

# Resume operations
kubectl scale deployment tuppr --replicas=1 -n system-upgrade
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

---

**⭐ If this project helps you, please consider giving it a star!**

For questions, issues, or feature requests, please visit our [GitHub Issues](https://github.com/home-operations/tuppr/issues).
