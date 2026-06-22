# tuppr - Talos Linux Upgrade Controller

A Kubernetes controller for managing automated upgrades of Talos Linux and Kubernetes.

## ✨ Features

### Core Capabilities

- 🚀 **Automated Talos node upgrades** with intelligent orchestration
- 🎯 **Kubernetes upgrades** - upgrade Kubernetes to newer versions
- 🔒 **Safe upgrade execution** - upgrades always run from healthy nodes (never self-upgrade)
- 📊 **Built-in health checks** - CEL-based expressions for custom cluster validation
- 🔄 **Configurable reboot modes** - default or powercycle options
- ⚡ **Parallel node upgrades** - configurable batch size to upgrade multiple nodes concurrently
- 📋 **Comprehensive status tracking** with real-time progress reporting
- ⚡ **Resilient job execution** with automatic retry and pod replacement
- 📈 **Prometheus metrics** - detailed monitoring of upgrade progress and health
- 🎯 **Per-node overrides** - use annotations to pin unique versions or switch installer flavors per node
- 🏷️ **Node labeling** - automatic labels during upgrades for integration with remediation systems
- 🚦 **Scheduling hints** - outdated nodes get a `PreferNoSchedule` taint to reduce workload churn during a rolling upgrade

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

Every chart value (controller, webhook, RBAC, and monitoring options) is
documented in the chart's generated README,
[`charts/tuppr/README.md`](charts/tuppr/README.md), built from
[`values.yaml`](charts/tuppr/values.yaml) — which also ships a
[`values.schema.json`](charts/tuppr/values.schema.json) for editor
autocompletion and `helm install`-time validation.

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
        version: v1.11.0 # Required - target Talos version

    policy:
        debug: true # Optional, verbose logging
        force: false # Optional, skip etcd health checks
        rebootMode: default # Optional, default|powercycle
        placement: hard # Optional, hard|soft (default hard)
        priorityClassName: system-node-critical # Optional, job pod priority class
        stage: false # Optional, stage upgrade
        timeout: 30m # Optional, per-node upgrade timeout

    # Custom health checks (optional)
    healthChecks:
        - apiVersion: v1
          kind: Node
          expr: status.conditions.exists(c, c.type == "Ready" && c.status == "True")

    # Talosctl configuration (optional)
    talosctl:
        image:
            repository: ghcr.io/siderolabs/talosctl # Optional, default
            tag: v1.11.0 # Optional, auto-detected
            pullPolicy: IfNotPresent # Optional, default

    # Maintenance windows (optional)
    maintenance:
        windows:
            - start: "0 2 * * 0" # Cron expression (Sunday 02:00)
              duration: "4h" # How long window stays open
              timezone: "UTC" # IANA timezone, default UTC

    # Node selector (optional)
    nodeSelector:
        matchExpressions:
            # Only upgrade nodes that have opted-in via this label
            - { key: tuppr.home-operations.com/upgrade, operator: In, values: ["enabled"] }
            # Exclude control plane nodes from this specific plan
            - { key: node-role.kubernetes.io/control-plane, operator: DoesNotExist }

    # Parallelism controls how many nodes are upgraded concurrently (optional)
    # Defaults to 1 (sequential). Must be >= 1 and <= number of matching nodes.
    parallelism: 1

    # Drain each node before it reboots for the upgrade (optional)
    drain:
        enabled: true

        # Optional: force delete instead of eviction
        # disableEviction: false
```

> [!NOTE]
> **Single-node clusters.** With only one node, the upgrade pod runs on the node
> being upgraded and is killed by the reboot. tuppr handles this: it issues the
> upgrade with `--wait=false` and `--drain=false`, and tracks completion by polling
> node readiness over the Talos API (so a reboot-killed Job isn't a failure). The
> drain is disabled because talosctl's default cordon+evict runs from inside this
> pod and would evict it before the reboot is issued — stranding the only node
> cordoned on the old version — and there is nowhere to drain to anyway. tuppr still
> uncordons the node after a verified upgrade even without a `drain` spec, and the
> controller tolerates the cordon taint so it can run there to do this.

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
        version: v1.36.2 # Required - target Kubernetes version

        # Optional - private registry for component images
        # (kube-apiserver, kube-controller-manager, kube-scheduler, kube-proxy, kubelet)
        # imageRepository: registry.example.com/k8s

    # Custom health checks (optional)
    healthChecks:
        - apiVersion: v1
          kind: Node
          expr: status.conditions.exists(c, c.type == "Ready" && c.status == "True")
          timeout: 10m

    # Talosctl configuration (optional)
    talosctl:
        image:
            repository: ghcr.io/siderolabs/talosctl # Optional, default
            tag: v1.11.0 # Optional, auto-detected
            pullPolicy: IfNotPresent # Optional, default

    # Maintenance windows (optional)
    maintenance:
        windows:
            - start: "0 2 * * 0" # Cron expression (Sunday 02:00)
              duration: "4h" # How long window stays open
              timezone: "UTC" # IANA timezone, default UTC
```

> Only one `KubernetesUpgrade` is allowed per cluster (admission webhook enforced).
> To upgrade again, edit `spec.kubernetes.version` on the existing resource.
> Past runs are recorded in `.status.history[]` (capped at 10, newest first) with
> `.status.startedAt` / `.status.completedAt`, and phase transitions are emitted as
> Kubernetes Events (`kubectl describe kubernetesupgrade kubernetes`). `TalosUpgrade`
> exposes the same fields plus per-run `completedNodes` / `failedNodes` snapshots.

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

    # Check deployments selected by labels
    - apiVersion: apps/v1
      kind: Deployment
      namespace: production
      labelSelector:
          matchLabels:
              app.kubernetes.io/part-of: critical-platform
          matchExpressions:
              - key: app.kubernetes.io/component
                operator: In
                values: ["api", "worker"]
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
    placement: hard # or "soft"

    # Use powercycle reboot for problematic nodes
    rebootMode: powercycle # or "default"

    # Stage upgrade then reboot to apply (2 total reboots)
    stage: false
```

### Parallel Upgrades (TalosUpgrade only)

By default, tuppr upgrades nodes one at a time (sequential). Setting `spec.parallelism` upgrades up to that many nodes concurrently within each batch:

```yaml
spec:
    talos:
        version: v1.11.0
    parallelism: 3 # upgrade up to 3 nodes at once
```

Constraints enforced by the admission webhook:

- Must be `>= 1`
- Cannot exceed the number of nodes matched by `spec.nodeSelector`

When `parallelism > 1`:

- Health checks run once before each batch, not per-node
- Drain (if configured) runs on all batch nodes before any upgrade job is created
- The batch waits for all node jobs to finish before starting the next batch
- Any failure in the batch stops further batches
- `status.currentNodes` lists all nodes in the active batch

### Pre/Post-Upgrade Hooks (TalosUpgrade only)

Run side-effecting Jobs around an upgrade run — e.g. set/unset Ceph `noout` so brief node reboots don't trigger PG rebalancing:

```yaml
spec:
    talos:
        version: v1.12.7
    hooks:
        pre:
            - name: ceph-set-noout
              image: ghcr.io/rook/rook:v1.18.7
              command: ["sh", "-c"]
              args: ["ceph osd set noout"]
              envFrom:
                  - secretRef:
                        name: rook-ceph-mon
              volumeMounts:
                  - name: ceph-config
                    mountPath: /etc/ceph
              volumes:
                  - name: ceph-config
                    secret:
                        secretName: rook-ceph-config
        post:
            - name: ceph-unset-noout
              image: ghcr.io/rook/rook:v1.18.7
              command: ["sh", "-c"]
              args: ["ceph osd unset noout"]
              envFrom:
                  - secretRef:
                        name: rook-ceph-mon
              volumeMounts:
                  - name: ceph-config
                    mountPath: /etc/ceph
              volumes:
                  - name: ceph-config
                    secret:
                        secretName: rook-ceph-config
```

Behavior:

- **Pre-hooks** run sequentially after the initial health check, before any node is touched. If any pre-hook fails, the upgrade is skipped and the run is marked `Failed` (post-hooks still run as cleanup).
- **Post-hooks** run sequentially after the upgrade reaches a terminal state (success or failure). They always run if any pre-hook was attempted. Post-hook failures are logged and recorded but don't override the upgrade outcome.
- **Inter-batch health checks are suppressed** while pre-hooks are configured. The contract: pre-hooks own cluster state for the upgrade window. Without pre-hooks, the per-batch re-check stays on (existing behavior).
- Each hook runs as a Job in the controller namespace with the same non-root, capabilities-dropped security posture as the upgrade Job. Mount your own credentials via `volumes` / `envFrom` and pick a `serviceAccountName` if you need cluster-API access.

Phase progression with hooks: `Pending → HealthChecking → PreHook → (Draining → Upgrading → Rebooting per batch) → PostHook → Completed`.

### Maintenance Windows

Control when upgrades start using cron-based maintenance windows. Running upgrades always complete without interruption.

```yaml
maintenance:
    windows:
        - start: "0 2 * * 0" # Sunday 02:00
          duration: "4h" # Max 168h, warn if <1h
          timezone: "Europe/Paris" # IANA timezone, default UTC
```

- Upgrades only start during open windows (stays `Pending` otherwise)
- Multiple windows create union (any open window allows start)
- In-progress upgrades always complete (never interrupted)
- TalosUpgrade re-checks between nodes
- Empty config: upgrades start immediately (backwards compatible)

### Per-Node Overrides

Tuppr supports overriding the global TalosUpgrade configuration on a per-node basis using Kubernetes annotations. This is useful for testing new versions on a canary node or handling nodes with different hardware schematics.

| Annotation                            | Description                                                                                                                                                                                                  | Example                         |
| ------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------- |
| tuppr.home-operations.com/version     | Overrides the target Talos version for this node.                                                                                                                                                            | v1.12.1                         |
| tuppr.home-operations.com/factory-url | Switch the node's installer flavor on the next upgrade (e.g. migrate from generic to factory, or from one flavor to another). Paired with the schematic that Talos reports at runtime via `ExtensionStatus`. | factory.talos.dev/aws-installer |
| tuppr.home-operations.com/schematic   | Companion to `factory-url`, only needed when migrating a node that has no runtime schematic yet (e.g. a freshly-joined node still on `ghcr.io/siderolabs/installer`). Ignored otherwise.                     | b55fbf...                       |

Example: Applying an override

```Bash
# Upgrade a specific node to a different version than the global policy
kubectl annotate node worker-01 tuppr.home-operations.com/version="v1.12.1"

# Switch a node from the generic installer to a factory flavor at the next upgrade
kubectl annotate node hcloud-01 \
  tuppr.home-operations.com/factory-url="factory.talos.dev/hcloud-installer" \
  tuppr.home-operations.com/schematic="314b18a3f89d..."
```

#### Image base resolution

Tuppr derives the upgrade image directly from each node's runtime state and `.machine.install.image`. Identical handling for hcloud / aws / metal — no platform branching.

Resolution per node:

1. **`tuppr.home-operations.com/factory-url` override** — when set, tuppr builds `<factory-url>/<schematic>:<target-version>`. The schematic comes from the runtime `ExtensionStatus` (the virtual `schematic` extension that Image Factory appends to every model), falling back to `tuppr.home-operations.com/schematic` if the runtime doesn't have one yet (first-time migration off the generic installer).
2. **Default** — version-swap the node's current `.machine.install.image`. A factory install stays on its factory base + schematic; a private registry path is preserved; a vanilla generic install stays vanilla.
3. **Safety net** — refused with a clear error when:
    - the runtime schematic doesn't appear in the install-image path (install-image and the running system disagree about which extensions are installed), or
    - the install image is the canonical generic Sidero installer (or a `/siderolabs/installer` mirror of it) AND the node has system extensions installed (reinstalling would silently wipe them).

    Both error messages point at the `factory-url` annotation as the fix.

## ⚠️ Safe Talos Upgrade Paths

Talos Linux has specific [supported upgrade paths](https://www.talos.dev/latest/talos-guides/upgrading-talos/#supported-upgrade-paths). You should always upgrade through each minor version sequentially rather than skipping minor versions. For example, upgrading from Talos v1.0 to v1.2.4 requires:

1. Upgrade from v1.0.x to the **latest patch** of v1.0 (e.g., v1.0.6)
2. Upgrade from v1.0.6 to the **latest patch** of v1.1 (e.g., v1.1.2)
3. Upgrade from v1.1.2 to v1.2.4

Tuppr does **not** automatically enforce safe upgrade paths — it will upgrade directly to whatever version you specify in the `TalosUpgrade` resource. It is your responsibility to ensure the target version is a valid upgrade from your current version.

### Recommended: Use Renovate for Safe Version Bumps

[Renovate](https://docs.renovatebot.com/) can automate version updates in your GitOps repository while respecting safe upgrade boundaries. Configure it to separate major/minor and minor/patch PRs so you can step through each version sequentially:

```json
{
    "packageRules": [
        {
            "matchDatasources": ["docker"],
            "matchPackageNames": ["ghcr.io/siderolabs/installer"],
            "separateMajorMinor": true,
            "separateMinorPatch": true
        }
    ]
}
```

- [`separateMajorMinor`](https://docs.renovatebot.com/configuration-options/#separatemajorminor) — creates separate PRs for major vs minor bumps
- [`separateMinorPatch`](https://docs.renovatebot.com/configuration-options/#separateminorpatch) — creates separate PRs for minor vs patch bumps

This way, Renovate will propose incremental version bumps that you can merge one at a time, ensuring you follow the supported upgrade path. Combine this with the `renovate` comment in your `TalosUpgrade` spec:

```yaml
spec:
    talos:
        # renovate: datasource=docker depName=ghcr.io/siderolabs/installer
        version: v1.11.0
```

## 📊 Monitoring & Metrics

Tuppr exposes Prometheus metrics under the `tuppr_` prefix. Hit `/metrics` on the controller pod for the authoritative list — keeping a copy here would drift.

The Helm chart wires up the standard observability stack on demand:

- `monitoring.serviceMonitor.enabled: true` — `ServiceMonitor` for Prometheus Operator scraping.
- `monitoring.prometheusRule.enabled: true` — bundled `PrometheusRule` with stuck-upgrade, failed-upgrade, and operator-absent alerts (see `charts/tuppr/templates/prometheusrule.tpl`).
- `monitoring.dashboards.enabled: true` — Grafana dashboard ConfigMap (sidecar-discoverable). Set `monitoring.dashboards.grafanaOperator.enabled: true` and `matchLabels` to also render a `GrafanaDashboard` CR for grafana-operator.

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

# Force a node to a specific version
kubectl annotate node <node-name> tuppr.home-operations.com/version="v1.10.7"

# Check if a node has overrides applied
kubectl get nodes -o custom-columns=NAME:.metadata.name,VERSION-OVERRIDE:.metadata.annotations."tuppr\.home-operations\.com/version"

# Check metrics endpoint
kubectl port-forward -n system-upgrade deployment/tuppr 8081:8081
curl http://localhost:8081/metrics | grep tuppr_
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

### Retrying a Failed Upgrade

`Failed` is terminal — the controller stops reconciling the upgrade until you take action. Three ways to retry:

```bash
# Reset annotation: wipes runtime state (phase, completedNodes, failedNodes,
# hook progress), keeps the spec, restarts from scratch.
kubectl annotate talosupgrade talos tuppr.home-operations.com/reset="$(date)"
kubectl annotate kubernetesupgrade kubernetes tuppr.home-operations.com/reset="$(date)"

# Spec edit: any change to .spec bumps generation and restarts the upgrade.
kubectl edit talosupgrade talos
kubectl edit kubernetesupgrade kubernetes

# Delete + recreate: loses history. Use only if the CR itself is corrupt.
kubectl delete talosupgrade talos && kubectl apply -f talos-upgrade.yaml
```

If the upgrade keeps reaching `Completed` but a node never catches up to the target version, the controller marks the run `Failed` after 5 completion cycles with a message like _"Node(s) never converged to v1.34.0 after 5 completion cycles"_. Investigate the lagging node before retrying.

### Troubleshooting

```bash
# Check job logs
kubectl logs job/tuppr-xyz -n system-upgrade

# Check controller health
kubectl get pods -n system-upgrade -l app.kubernetes.io/name=tuppr

# View metrics for debugging
kubectl port-forward -n system-upgrade deployment/tuppr 8081:8081
curl http://localhost:8081/metrics | grep -E "(tuppr_.*_phase|tuppr_.*_duration)"
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

| Feature                  | TalosUpgrade                                                                                                 | KubernetesUpgrade            |
| ------------------------ | ------------------------------------------------------------------------------------------------------------ | ---------------------------- |
| **Scope**                | Talos nodes                                                                                                  | Kubernetes cluster           |
| **Multiple CRs**         | ✅ Multiple allowed (queued)                                                                                 | ❌ Only one per cluster      |
| **Execution**            | Sequential or parallel within a plan (configurable via `spec.parallelism`); only one plan executes at a time | Single controller node       |
| **Reboot Required**      | ✅ Yes                                                                                                       | ❌ No                        |
| **Health Checks**        | ✅ Before each node                                                                                          | ✅ Before upgrade            |
| **Concurrent Execution** | ❌ Blocked by other upgrades                                                                                 | ❌ Blocked by other upgrades |
| **Handling Failures**    | ❌ Manual                                                                                                    | ❌ Manual                    |
| **Metrics**              | ✅ Comprehensive                                                                                             | ✅ Comprehensive             |

### Important Resource Constraints

- **TalosUpgrade**: Multiple `TalosUpgrade` resources are allowed per cluster and can target different groups of nodes (for example, "workers-west" vs "workers-east"). However, only one `TalosUpgrade` plan executes at a time on a first-come, first-served basis. The controller queues subsequent plans to ensure safe, sequential orchestration across the cluster. Within a single plan, use `spec.parallelism` to upgrade multiple nodes concurrently.

- **KubernetesUpgrade**: Only **one** `KubernetesUpgrade` resource is allowed per cluster. This constraint exists because Kubernetes upgrades affect the entire cluster, and multiple concurrent upgrades would conflict with each other. The admission webhook will reject attempts to create additional `KubernetesUpgrade` resources.

- **Cross-Upgrade Coordination**: TalosUpgrade and KubernetesUpgrade resources **cannot run concurrently**. If one upgrade is in progress (status.phase == "InProgress"), the other will wait in a "Pending" state until the active upgrade completes. This prevents conflicts between Talos node changes and Kubernetes cluster changes that could destabilize the cluster.

### Upgrade Coordination Examples

```yaml
# ✅ Valid: Multiple TalosUpgrade Plans (Queued Execution)
# Plan 1: Upgrade worker nodes in west zone
apiVersion: tuppr.home-operations.com/v1alpha1
kind: TalosUpgrade
metadata:
    name: workers-west
spec:
    talos:
        version: v1.12.4
    nodeSelector:
        matchLabels:
            topology.kubernetes.io/zone: west
---
# Plan 2: Upgrade worker nodes in east zone
apiVersion: tuppr.home-operations.com/v1alpha1
kind: TalosUpgrade
metadata:
    name: workers-east
spec:
    talos:
        version: v1.12.4
    nodeSelector:
        matchLabels:
            topology.kubernetes.io/zone: east
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
    name: another-kubernetes # This will fail validation
spec:
    kubernetes:
        version: v1.35.0
```

#### ⚠️ Warning: Node Overlap

If two active plans target the same node (e.g., Plan A selects role: worker and Plan B selects zone: west, and a node has both labels), the webhook will issue a Warning upon creation. While allowed, this configuration is discouraged as it can cause conflicting upgrade cycles where a node is repeatedly updated by alternating plans.

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
