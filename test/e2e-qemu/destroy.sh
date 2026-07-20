#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "${SCRIPT_DIR}/common.sh"

log "Destroying cluster $CLUSTER_NAME"
sudo -E "$(talosctl_bin)" cluster destroy --name "$CLUSTER_NAME" --force
rm -rf "$CONFIG_DIR"
