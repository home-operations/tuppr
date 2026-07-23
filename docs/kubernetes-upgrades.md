# Kubernetes upgrades

A `KubernetesUpgrade` sets the target Kubernetes version for the cluster. Unlike
a Talos upgrade it does not reboot nodes, and there is **exactly one** of these
resources per cluster.

```yaml
apiVersion: tuppr.home-operations.com/v1alpha1
kind: KubernetesUpgrade
metadata:
  name: kubernetes
spec:
  kubernetes:
    # renovate: datasource=docker depName=ghcr.io/siderolabs/kubelet
    version: v1.36.3

    # Optional: pull control-plane component images (kube-apiserver,
    # kube-controller-manager, kube-scheduler, kube-proxy, kubelet) from a
    # private registry.
    # imageRepository: registry.example.com/k8s

  # Optional health checks - same schema as TalosUpgrade.
  healthChecks:
    - apiVersion: v1
      kind: Node
      expr: status.conditions.exists(c, c.type == "Ready" && c.status == "True")
      timeout: 10m

  # Optional maintenance windows and talosctl image, identical to TalosUpgrade.
```

The optional fields - [health checks](talos-upgrades.md#health-checks),
[maintenance windows](talos-upgrades.md#maintenance-windows), and the
[`talosctl` image](talos-upgrades.md#talosctl-image) - work exactly as they do
for a Talos upgrade.

## One per cluster

A Kubernetes upgrade is cluster-wide, so the admission webhook allows only one
`KubernetesUpgrade` and rejects a second. To upgrade again, edit
`spec.kubernetes.version` on the existing resource:

```bash
kubectl edit kubernetesupgrade kubernetes
```

## History and status

Each run records `.status.startedAt` / `.status.completedAt`, and past runs are
kept in `.status.history[]` (newest first, capped at 10). Phase transitions are
emitted as Kubernetes Events:

```bash
kubectl describe kubernetesupgrade kubernetes
```

A `KubernetesUpgrade` never runs at the same time as a `TalosUpgrade`; see
[Upgrade coordination](coordination.md).
