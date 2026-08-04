# tuppr

A Kubernetes controller for automated, orchestrated upgrades of **Talos Linux**
and **Kubernetes**. Declare a target version in a custom resource; tuppr plans
and executes the rollout - draining, upgrading, rebooting, and health-checking
each node in turn (or in parallel batches) - always driving the upgrade from a
healthy node, so it never self-upgrades the node it runs on.

📖 **Docs site: <https://tuppr.home-operations.com/>** - requirements,
quickstart, upgrade coordination, Talos and Kubernetes upgrade options,
notifications (Apprise + chaski), monitoring, operations, and chart values.

## When to use it

- You run Talos Linux and want version upgrades driven by the Kubernetes API
  (GitOps-friendly) instead of running `talosctl upgrade` by hand.
- You want the rollout orchestrated: health-gated, one plan at a time, with
  drain, reboot, and node-readiness verification handled for you.
- You want to keep the Kubernetes version in step through the same declarative
  flow.

## When _not_ to use it

- You need tuppr to pick a _safe_ version for you. It upgrades to exactly the
  version you specify and does not enforce Talos's sequential upgrade path -
  that is your responsibility
  ([Versioning](https://tuppr.home-operations.com/versioning/)).
- You are not on Talos. tuppr drives upgrades over the Talos API; it is not a
  general-purpose node OS upgrader.

## The two resources

tuppr manages two kinds of upgrade in the `tuppr.home-operations.com/v1alpha1`
API group. Only one upgrade ever runs at a time cluster-wide: multiple
`TalosUpgrade` plans queue, and the two kinds never run concurrently (see
[Upgrade coordination](https://tuppr.home-operations.com/coordination/)).

| Resource            | Upgrades               | Reboot | Per cluster                    |
| ------------------- | ---------------------- | ------ | ------------------------------ |
| `TalosUpgrade`      | Talos Linux on nodes   | Yes    | Many (queued), node-selectable |
| `KubernetesUpgrade` | The Kubernetes version | No     | Exactly one                    |

## Quickstart

Grant the controller's namespace `os:admin` Talos API access (apply to every
node), then install the chart. Full prerequisites:
[Requirements](https://tuppr.home-operations.com/requirements/).

```yaml
# machine config, on every node
machine:
  features:
    kubernetesTalosAPIAccess:
      enabled: true
      allowedRoles: [os:admin]
      allowedKubernetesNamespaces: [system-upgrade]
```

```bash
helm install tuppr oci://ghcr.io/home-operations/charts/tuppr \
  --namespace system-upgrade
```

Upgrade Talos (rolls the version across matching nodes, health-gated):

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

Upgrade Kubernetes (one resource per cluster; edit the version to upgrade
again):

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

Then `kubectl get talosupgrade -w`. Health checks, parallel batches, hooks,
maintenance windows, and per-node overrides:
[Talos upgrades](https://tuppr.home-operations.com/talos-upgrades/).

## Development

Tooling is pinned with [mise](https://mise.jdx.dev): `mise run test`,
`mise run lint`, `mise run build`, `mise run manifests`. The docs site builds
with `mise run docs` (strict link checking) and serves locally with
`mise run docs-serve`. See
[Development](https://tuppr.home-operations.com/development/).

## License

[AGPL-3.0](LICENSE). Inspired by
[Talos Linux](https://www.talos.dev/) and the
[System Upgrade Controller](https://github.com/rancher/system-upgrade-controller).
