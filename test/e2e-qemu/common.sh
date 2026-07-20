#!/usr/bin/env bash
# Shared configuration for the QEMU e2e scripts. Meant to be sourced.
# shellcheck disable=SC2034  # consumers of this file use these

# What the cluster boots on is declared in the leg documents (<cp>cp-<w>w.yaml);
# these are what tuppr upgrades it to, and have to stay ahead of them or the run
# proves nothing.
TALOS_UPGRADE_VERSION="${TALOS_UPGRADE_VERSION:-v1.13.6}"
K8S_UPGRADE_VERSION="${K8S_UPGRADE_VERSION:-v1.35.0}"

log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}
