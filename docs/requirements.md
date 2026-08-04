# Requirements

## Cluster

- A **Talos Linux** cluster with the Talos API reachable from the controller.
- A **namespace** for the controller (the examples use `system-upgrade`).

## Talos API access

The controller upgrades nodes over the Talos API, so the namespace it runs in
must be granted `os:admin` access. Apply this machine config to **every** node
(control plane and workers):

```yaml
machine:
  features:
    kubernetesTalosAPIAccess:
      enabled: true
      allowedRoles:
        - os:admin
      allowedKubernetesNamespaces:
        - system-upgrade # the namespace tuppr is installed into
```

/// warning | Namespace must match
`allowedKubernetesNamespaces` must list the exact namespace tuppr runs in. If it
does not, upgrade jobs cannot authenticate to the Talos API and every upgrade
fails at the first node.
///

## Versions

- tuppr upgrades Talos to exactly the version you set in a
  [`TalosUpgrade`](talos-upgrades.md); it does **not** enforce Talos's supported
  upgrade path. Step through minor versions yourself - see
  [Versioning and safe upgrade paths](versioning.md).
- The upgrade Job uses `talosctl`; its version is auto-detected but can be
  pinned per resource (`spec.talosctl.image`).

## Next

Install the chart and run your first upgrade in the
[Quickstart](quickstart.md).
