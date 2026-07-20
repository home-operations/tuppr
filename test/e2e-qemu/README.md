# QEMU E2E Tests

End-to-end tests for the tuppr controller against a local Talos cluster
provisioned by `talosctl cluster create qemu`.

## Overview

`cluster.sh` boots a Talos cluster in QEMU VMs, `test.sh` deploys the
controller and drives a real TalosUpgrade and KubernetesUpgrade against it. The
control plane sits behind the provisioner's built-in load balancer, so the
Kubernetes API stays reachable while tuppr reboots control plane nodes one at a
time.

## Requirements

- KVM (`/dev/kvm`) and `qemu-system-x86_64`
- Passwordless `sudo`, for the bridge and NAT the provisioner sets up
- Docker, to build and push the controller image (skipped if you set
  `CONTROLLER_IMAGE`)

## Quick Start

```fish
./cluster.sh
./test.sh
./destroy.sh
```

`test.sh` picks up the kubeconfig and talosconfig that `cluster.sh` wrote, so
the only thing worth exporting is a prebuilt image if you have one:

```fish
set -gx CONTROLLER_IMAGE ghcr.io/you/tuppr:dev
```

When `CONTROLLER_IMAGE` is unset, `test.sh` builds and pushes one itself via
`image.sh`. CI instead sets the tag up front and runs `image.sh` concurrently
with `cluster.sh`, since the build and the VMs share no inputs; `test.sh` then
finds the image already built and skips straight to installing it.

## Configuration

Every knob is an environment variable read by `common.sh`, and both scripts
must see the same values.

| Variable                  | Default              | Description                             |
| ------------------------- | -------------------- | --------------------------------------- |
| `CONTROL_PLANE_COUNT`     | `3`                  | Control plane VMs                       |
| `WORKER_COUNT`            | `0`                  | Worker VMs                              |
| `TALOS_BOOTSTRAP_VERSION` | `v1.12.4`            | Talos version the cluster boots on      |
| `TALOS_UPGRADE_VERSION`   | `v1.12.6`            | Talos version tuppr upgrades to         |
| `K8S_BOOTSTRAP_VERSION`   | `v1.34.0`            | Kubernetes version the cluster boots on |
| `K8S_UPGRADE_VERSION`     | `v1.35.0`            | Kubernetes version tuppr upgrades to    |
| `CONTROLPLANE_MEMORY`     | `2GiB`               | Memory per control plane VM             |
| `CLUSTER_CIDR`            | `10.5.0.0/24`        | Cluster network                         |
| `CONFIG_DIR`              | `/tmp/$CLUSTER_NAME` | Where kubeconfig and talosconfig land   |

The cluster is named `tuppr-e2e-{cp}cp-{w}w`, so a 3cp/0w run is
`tuppr-e2e-3cp-0w`. Keep that name short: it appears twice in the QEMU monitor
socket path, and QEMU refuses to start when a UNIX socket path exceeds 108
bytes.

`test.sh` refuses to run unless the kubeconfig path, the kubectl context, and
the node names all look like an e2e cluster, so it cannot be pointed at a real
one by accident.

## How the machine config is built

`cluster.sh` POSTs `schematic.yaml` to the Image Factory and uses the returned
id for both the boot media and `.machine.install.image`. That pairing matters:
nodes booted from a Factory image report a schematic at runtime, and tuppr
refuses to upgrade a node whose install image does not embed that same
schematic, because reinstalling would wipe its extensions. `talosctl cluster
create qemu` does not set an install image on its own, so `patches/all.yaml`
supplies it.

`patches/controlplane.yaml` enables `kubernetesTalosAPIAccess`, which is how
the controller reaches the Talos API from inside the cluster. Talos rejects
that feature on workers, so it is applied only to control planes.
