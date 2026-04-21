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

# Order matters - delete dependent resources first.
# Each entry is "<cmd>:<extra list flags>". The extra flags come after `list`.
RESOURCE_TYPES=(
    "server:"
    "load-balancer:"
    "firewall:"
    "network:"
    "placement-group:"
    "primary-ip:"
    "floating-ip:"
    "ssh-key:"
    # Snapshots (uploaded by imager_image). -t snapshot ensures we never match
    # a public image even if labels somehow collided.
    "image:-t snapshot"
)

for entry in "${RESOURCE_TYPES[@]}"; do
    cmd=${entry%%:*}
    list_args=${entry#*:}
    log "Cleaning up ${cmd}s..."
    # shellcheck disable=SC2086 # intentional word splitting for list_args
    ids=$(hcloud "$cmd" list $list_args -o noheader -o columns=id -l "${LABEL_SELECTOR}" 2>/dev/null || true)

    if [[ -n "$ids" ]]; then
        echo "$ids" | while read -r id; do
            if [[ -n "$id" ]]; then
                log "  Deleting $cmd $id"
                hcloud "$cmd" delete "$id" || log "  WARNING: Failed to delete $cmd $id"
            fi
        done
    else
        log "  No ${cmd}s found"
    fi
done

log "Cleanup complete for label selector: ${LABEL_SELECTOR}"
