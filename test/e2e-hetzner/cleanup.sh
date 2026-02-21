#!/usr/bin/env bash
set -euo pipefail

LABEL_SELECTOR="${1:-}"

if [[ -z "$LABEL_SELECTOR" ]]; then
    echo "Usage: $0 <label_selector>"
    echo ""
    echo "Examples:"
    echo "  $0 cluster=tuppr-e2e-local-3cp-0w"
    echo "  $0 branch=main"
    echo ""
    echo "Deletes all Hetzner Cloud resources matching the label selector"
    exit 1
fi

if [[ -z "${HCLOUD_TOKEN:-}" ]]; then
    echo "ERROR: HCLOUD_TOKEN environment variable not set"
    exit 1
fi

log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

log "Cleaning up all resources with label selector: ${LABEL_SELECTOR}"

# Order matters - delete dependent resources first
RESOURCE_TYPES=(
    "server"
    "load-balancer"
    "firewall"
    "network"
    "placement-group"
    "primary-ip"
    "floating-ip"
    "ssh-key"
)

for resource in "${RESOURCE_TYPES[@]}"; do
    log "Cleaning up ${resource}s..."
    ids=$(hcloud "$resource" list -o noheader -o columns=id -l "${LABEL_SELECTOR}" 2>/dev/null || true)

    if [[ -n "$ids" ]]; then
        echo "$ids" | while read -r id; do
            if [[ -n "$id" ]]; then
                log "  Deleting $resource $id"
                hcloud "$resource" delete "$id" || log "  WARNING: Failed to delete $resource $id"
            fi
        done
    else
        log "  No ${resource}s found"
    fi
done

log "Cleanup complete for label selector: ${LABEL_SELECTOR}"
