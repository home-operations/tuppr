#!/usr/bin/env bash
# Shared configuration for the QEMU e2e scripts. Meant to be sourced.
# shellcheck disable=SC2034  # consumers of this file use these

CONTROL_PLANE_COUNT="${CONTROL_PLANE_COUNT:-3}"
WORKER_COUNT="${WORKER_COUNT:-0}"

TALOS_BOOTSTRAP_VERSION="${TALOS_BOOTSTRAP_VERSION:-v1.12.4}"
TALOS_UPGRADE_VERSION="${TALOS_UPGRADE_VERSION:-v1.12.6}"
K8S_BOOTSTRAP_VERSION="${K8S_BOOTSTRAP_VERSION:-v1.34.0}"
K8S_UPGRADE_VERSION="${K8S_UPGRADE_VERSION:-v1.35.0}"

# Measured on an idle control plane: 694MB in use, peaking at 795MB during an
# upgrade (the installer streams to disk rather than buffering in RAM). 2GiB
# leaves room for the test workload without over-reserving on a 16GB runner.
CONTROLPLANE_MEMORY="${CONTROLPLANE_MEMORY:-2GiB}"

CLUSTER_CIDR="${CLUSTER_CIDR:-10.5.0.0/24}"

# Keep this short. It lands in the QEMU monitor socket path twice, as both the
# state directory and the node name prefix, and a UNIX socket path over 108
# bytes makes QEMU refuse to start:
#   ~/.talos/clusters/<name>/<name>-controlplane-N.monitor
# There is nothing to disambiguate here anyway: the cluster is local to the
# machine, and the CI legs each get their own runner.
CLUSTER_NAME="tuppr-e2e-${CONTROL_PLANE_COUNT}cp-${WORKER_COUNT}w"
CONFIG_DIR="${CONFIG_DIR:-/tmp/${CLUSTER_NAME}}"

export KUBECONFIG="${CONFIG_DIR}/kubeconfig"
export TALOSCONFIG="${CONFIG_DIR}/talosconfig"

# The provisioner puts the host end of the bridge on the first address of the
# CIDR and numbers nodes upward from the second.
_cidr_addr="${CLUSTER_CIDR%%/*}"
CLUSTER_GATEWAY="${_cidr_addr%.*}.1"
FIRST_CONTROLPLANE_IP="${_cidr_addr%.*}.2"

log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

# "mise which" resolves past the shim, which does not survive sudo.
talosctl_bin() {
    mise which talosctl 2>/dev/null || command -v talosctl
}
