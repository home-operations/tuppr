#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "${SCRIPT_DIR}/common.sh"

main() {
    log "Creating cluster $CLUSTER_NAME (${CONTROL_PLANE_COUNT}cp/${WORKER_COUNT}w)"
    log "Talos $TALOS_BOOTSTRAP_VERSION, Kubernetes $K8S_BOOTSTRAP_VERSION"

    mkdir -p "$CONFIG_DIR"

    # The schematic id is a content hash, so re-POSTing an unchanged schematic
    # is idempotent.
    local schematic_id
    schematic_id=$(curl -sfX POST https://factory.talos.dev/schematics \
        --data-binary "@${SCRIPT_DIR}/schematic.yaml" | jq -r '.id // empty')
    if [[ -z "$schematic_id" ]]; then
        log "ERROR: could not resolve a schematic id from the Image Factory"
        exit 1
    fi
    log "Factory schematic: $schematic_id"

    SCHEMATIC_ID="$schematic_id" TALOS_BOOTSTRAP_VERSION="$TALOS_BOOTSTRAP_VERSION" \
        CLUSTER_GATEWAY="$CLUSTER_GATEWAY" \
        envsubst < "${SCRIPT_DIR}/patches/all.yaml" > "${CONFIG_DIR}/patch-all.yaml"

    # The provisioner drops generated machine configs, which embed cluster PKI,
    # into the working directory.
    cd "$CONFIG_DIR"

    # Root is needed for the bridge and NAT. Cluster state stays in the default
    # ~/.talos/clusters: the VM monitor sockets live alongside it and a longer
    # path overflows the 108-byte UNIX socket limit.
    sudo -E "$(talosctl_bin)" cluster create qemu \
        --name "$CLUSTER_NAME" \
        --cidr "$CLUSTER_CIDR" \
        --schematic-id "$schematic_id" \
        --talos-version "$TALOS_BOOTSTRAP_VERSION" \
        --kubernetes-version "${K8S_BOOTSTRAP_VERSION#v}" \
        --controlplanes "$CONTROL_PLANE_COUNT" \
        --workers "$WORKER_COUNT" \
        --memory-controlplanes "$CONTROLPLANE_MEMORY" \
        --disks virtio:10GiB \
        --config-patch "@${CONFIG_DIR}/patch-all.yaml" \
        --config-patch-controlplanes "@${SCRIPT_DIR}/patches/controlplane.yaml" \
        --talosconfig-destination "$TALOSCONFIG"

    sudo chown -R "$(id -u):$(id -g)" "$CONFIG_DIR"

    talosctl kubeconfig "$KUBECONFIG" --nodes "$FIRST_CONTROLPLANE_IP" --force

    log "Cluster ready. KUBECONFIG=$KUBECONFIG TALOSCONFIG=$TALOSCONFIG"
}

main "$@"
