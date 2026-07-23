# Quickstart

This walks through installing tuppr and running one Talos and one Kubernetes
upgrade. It assumes you have granted Talos API access to the controller
namespace - see [Requirements](requirements.md).

## Install

```bash
helm install tuppr oci://ghcr.io/home-operations/charts/tuppr \
  --namespace system-upgrade
```

Every chart value is documented in the generated
[Helm chart values](configuration.md) reference.

## Upgrade Talos

Create a `TalosUpgrade`. This is the minimal form - one target version applied
to every node, sequentially:

```yaml
apiVersion: tuppr.home-operations.com/v1alpha1
kind: TalosUpgrade
metadata:
  name: cluster
spec:
  talos:
    # renovate: datasource=docker depName=ghcr.io/siderolabs/installer
    version: v1.13.7
```

tuppr health-checks the cluster, then upgrades each node in turn: drain →
`talosctl upgrade` → reboot → wait for the node to return `Ready` on the new
version. Watch it:

```bash
kubectl get talosupgrade -w
kubectl describe talosupgrade cluster
```

Adding health checks, parallel batches, hooks, maintenance windows, and
per-node overrides: [Talos upgrades](talos-upgrades.md).

/// note | Single-node clusters
With one node, the upgrade pod runs on the node being rebooted. tuppr handles
this automatically (issues the upgrade with `--wait=false`, skips the drain, and
tracks completion by polling node readiness over the Talos API), then uncordons
the node once the upgrade is verified.
///

## Upgrade Kubernetes

Create the (single) `KubernetesUpgrade`:

```yaml
apiVersion: tuppr.home-operations.com/v1alpha1
kind: KubernetesUpgrade
metadata:
  name: kubernetes
spec:
  kubernetes:
    # renovate: datasource=docker depName=ghcr.io/siderolabs/kubelet
    version: v1.36.3
```

```bash
kubectl get kubernetesupgrade -w
```

There is exactly one `KubernetesUpgrade` per cluster; to upgrade again, edit
`spec.kubernetes.version` on it. Details: [Kubernetes
upgrades](kubernetes-upgrades.md).

## Both at once

You can apply both resources; tuppr never runs them concurrently. Whichever
starts first runs to completion while the other waits in `Pending`, then the
second starts automatically. See [Upgrade coordination](coordination.md).
