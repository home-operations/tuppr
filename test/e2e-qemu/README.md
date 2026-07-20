# QEMU E2E Tests

End-to-end tests for the tuppr controller against a local Talos cluster, booted by
[talosctl-cluster-action][action] on top of `talosctl cluster create qemu`.

## Overview

Each CI leg boots a `TalosCluster` document from this directory and drives one
controller against it. The action boots the shape the document describes, exports
a kubeconfig and talosconfig, and destroys the cluster in its post step; `test.sh`
then deploys the controller and drives a real upgrade. The control plane sits
behind the provisioner's built-in load balancer, so the Kubernetes API stays
reachable while tuppr reboots control plane nodes one at a time.

| Leg      | Upgrade      | Shape                     | What only this leg covers                                  |
| -------- | ------------ | ------------------------- | ---------------------------------------------------------- |
| `1cp-0w` | `talos`      | 1 control plane           | tuppr upgrading the node it runs on: no drain, no `--wait` |
| `1cp-1w` | `talos`      | 1 control plane, 1 worker | a drained workload having somewhere to land                |
| `3cp-0w` | `talos`      | 3 control planes          | etcd quorum surviving a rolling reboot                     |
| `1cp-1w` | `kubernetes` | 1 control plane, 1 worker | both kinds of kubelet converging on the new version        |

One upgrade per leg, chosen by `UPGRADE_KIND`. Shape is what makes the Talos legs
differ: that controller drains, reboots, and upgrades node by node, so each shape
reaches code the others do not. `KubernetesUpgrade` picks a single control plane
node and runs one job there, with no drain and no reboot, so it earns one leg
rather than a shape sweep of its own. Nothing runs both, which means no leg covers
a Kubernetes upgrade layered on a Talos one, and a failure names the controller
that broke.

```text
1cp-0w.yaml, 1cp-1w.yaml, 3cp-0w.yaml   one leg per cluster shape
patches/registry.yaml                   shared: where the nodes pull the controller image
patches/talos-api-access.yaml           shared: how the controller reaches the Talos API
cr-templates/                           the TalosUpgrade and KubernetesUpgrade under test
```

## Requirements

- KVM (`/dev/kvm`) and `qemu-system-x86_64`
- Passwordless `sudo`, for the bridge and NAT the provisioner sets up
- `talosctl` v1.13 or newer, which mise pins
- Docker, to build and push the controller image (skipped if you set
  `CONTROLLER_IMAGE`)

## Running it locally

`test.sh` runs against a cluster that already exists; the action is only how CI
gets one. Boot the leg you want with `talosctl` directly, then point the script at
it:

```fish
talosctl cluster create qemu \
    --name tuppr-e2e-1cp-0w \
    --controlplanes 1 --workers 0 \
    --talos-version v1.13.5 --kubernetes-version 1.34.0 \
    --disks virtio:10GiB \
    --config-patch-controlplanes @patches/talos-api-access.yaml \
    --talosconfig-destination /tmp/tuppr-e2e/talosconfig

talosctl kubeconfig /tmp/tuppr-e2e/kubeconfig --nodes 10.5.0.2 --force

set -gx CLUSTER_NAME tuppr-e2e-1cp-0w
set -gx KUBECONFIG /tmp/tuppr-e2e/kubeconfig
set -gx TALOSCONFIG /tmp/tuppr-e2e/talosconfig
set -gx UPGRADE_KIND talos # or kubernetes
./test.sh

talosctl cluster destroy --name tuppr-e2e-1cp-0w --force
```

That is the leg document spelled out as flags, minus the schematic; a cluster with
no schematic takes tuppr's no-schematic fall-through rather than the matching guard
(see below), so it is a slightly weaker test than CI's. The documents stay the
source of truth for the shapes.

`test.sh` refuses to run unless `CLUSTER_NAME` looks like an e2e cluster and both
the kubectl context and the node names agree with it, so it cannot be pointed at a
real cluster by accident.

When `CONTROLLER_IMAGE` is unset, `test.sh` builds and pushes one itself via
`image.sh`, which uses ttl.sh so a local run needs no registry of its own:

```fish
set -gx CONTROLLER_IMAGE ghcr.io/you/tuppr:dev
```

CI instead fixes the tag up front and builds it concurrently with the cluster,
since the build and the VMs share no inputs; `test.sh` then finds the image already
built and skips straight to installing it. CI builds through
`docker/build-push-action` rather than `image.sh` so the layers land in the GitHub
Actions cache, which `image.sh` has no way to reach.

CI also runs its own registry on the runner and pushes there, so the image never
crosses the internet. The nodes reach it as `registry.e2e`, which the mirror in
`patches/registry.yaml` points at port 5000 on the QEMU bridge gateway. That mirror
entry is inert for a local run, where nothing references `registry.e2e`.

## Configuration

The versions a leg **boots** on are in its document, as `spec.qemu.talos-version`
and `spec.kubernetes-version`. The versions tuppr **upgrades it to** are in
`common.sh`, as `TALOS_UPGRADE_VERSION` and `K8S_UPGRADE_VERSION`, and both have to
stay ahead of the documents or the run proves nothing. Talos itself has a floor:
the action needs `talosctl` v1.13 or newer, so a leg cannot boot anything older
than v1.13 to upgrade from. Everything else about a shape, including the memory
ceiling that bounds how many VMs fit on a runner, lives in the document.

Keep `metadata.name` short: it appears twice in the QEMU monitor socket path, and
QEMU refuses to start when a UNIX socket path exceeds 108 bytes. The action checks
this before it provisions anything.

## What the action's profile supplies

The documents are small because the action's default `ephemeral` profile already
applies what a throwaway cluster wants, ahead of the patches here: the kernel args
that turn off the dashboard and auditd, kubelet's image GC and eviction thresholds
(so a full disk reads as itself rather than as a flaky test), etcd
`unsafe-no-fsync`, and an apiserver audit policy of `None`. Every run logs what it
applied.

Two of its effects are load-bearing for tuppr specifically, and both would
disappear under `profile: none`:

- The profile's kernel args are registered as an **Image Factory schematic**, so
  the nodes report a schematic id at runtime. That is what makes the run exercise
  the schematic-matching guard in `buildTalosUpgradeImage` rather than the
  no-schematic fall-through.
- Because a schematic is in play, the profile pins **`.machine.install.image`** to
  the matching Factory installer. `talosctl` never sets it, and tuppr refuses to
  upgrade a node whose install image does not embed the schematic the node booted
  with, because reinstalling would wipe its extensions.

What is left in `patches/` is only what the profile has no opinion about: where the
nodes pull the controller image from, and `kubernetesTalosAPIAccess`, which is how
the controller reaches the Talos API from inside the cluster. Talos rejects that
feature on workers, so it is a control plane patch.

[action]: https://github.com/home-operations/talosctl-cluster-action
