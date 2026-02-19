#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TERRAFORM_DIR="${SCRIPT_DIR}/tofu"
CR_TEMPLATES_DIR="${SCRIPT_DIR}/cr-templates"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

wait_for_cr_completed() {
    local kind=$1
    local name=$2
    local timeout_seconds=$3
    local deadline=$((SECONDS + timeout_seconds))

    log "Waiting for $kind/$name to complete (timeout: ${timeout_seconds}s)..."
    while [[ $SECONDS -lt $deadline ]]; do
        local phase
        phase=$(kubectl get "$kind" "$name" -o jsonpath='{.status.phase}' 2>/dev/null || echo "NotFound")

        log "$kind/$name phase: $phase"

        if [[ "$phase" == "Completed" ]]; then
            log "$kind/$name completed successfully"
            return 0
        fi

        if [[ "$phase" == "Failed" ]]; then
            log "ERROR: $kind/$name entered Failed phase"
            kubectl get "$kind" "$name" -o yaml
            return 1
        fi

        sleep 10
    done

    log "ERROR: Timeout waiting for $kind/$name to complete"
    kubectl get "$kind" "$name" -o yaml
    return 1
}

verify_talos_version() {
    local expected_version=$1
    log "Verifying all nodes are at Talos version $expected_version..."

    # Get node IPs (talosconfig doesn't have nodes field set by module)
    local node_ips
    node_ips=$(kubectl get nodes -o json | jq -r '[.items[].status.addresses[] | select(.type=="ExternalIP") | .address] | join(",")')

    local output
    output=$(talosctl version --short --nodes "$node_ips" 2>&1)

    local server_versions
    server_versions=$(echo "$output" | grep "Tag:" | awk '{print $2}' | sort -u)

    local version_count
    version_count=$(echo "$server_versions" | wc -l)

    if [[ $version_count -ne 1 ]]; then
        log "ERROR: Found multiple Talos versions across nodes:"
        echo "$output"
        return 1
    fi

    if [[ "$server_versions" != "$expected_version" ]]; then
        log "ERROR: Nodes are at $server_versions, expected $expected_version"
        echo "$output"
        return 1
    fi

    log "All nodes are at Talos version $expected_version"
    return 0
}

verify_k8s_version() {
    local expected_version=$1
    log "Verifying all nodes are at Kubernetes version $expected_version..."

    local nodes
    nodes=$(kubectl get nodes -o json)

    local node_count
    node_count=$(echo "$nodes" | jq -r '.items | length')

    for ((i=0; i<node_count; i++)); do
        local node_name
        node_name=$(echo "$nodes" | jq -r ".items[$i].metadata.name")
        local kubelet_version
        kubelet_version=$(echo "$nodes" | jq -r ".items[$i].status.nodeInfo.kubeletVersion")

        if [[ "$kubelet_version" != "$expected_version" ]]; then
            log "ERROR: Node $node_name kubelet is at $kubelet_version, expected $expected_version"
            return 1
        fi
        log "Node $node_name kubelet: $kubelet_version"
    done

    log "All nodes are at Kubernetes version $expected_version"
    return 0
}

main() {
    cd "$TERRAFORM_DIR"

    log "Reading Terraform outputs..."
    KUBECONFIG=$(tofu output -raw kubeconfig_path)
    TALOSCONFIG=$(tofu output -raw talosconfig_path)
    TALOS_TO_VERSION=$(tofu output -raw talos_upgrade_version)
    K8S_TO_VERSION=$(tofu output -raw k8s_upgrade_version)
    EXPECTED_CLUSTER=$(tofu output -raw cluster_name)

    log "Kubeconfig: $KUBECONFIG"
    log "Talosconfig: $TALOSCONFIG"

    # Safety checks before proceeding
    if [[ "$KUBECONFIG" != /tmp/tuppr-e2e-* ]]; then
        log "ERROR: Kubeconfig path '$KUBECONFIG' doesn't match expected pattern '/tmp/tuppr-e2e-*'"
        log "       Refusing to proceed to avoid touching non-e2e clusters"
        exit 1
    fi

    # Export after validation
    export KUBECONFIG
    export TALOSCONFIG

    # Verify we're connected to the right cluster
    local current_context
    current_context=$(kubectl config current-context)
    if [[ "$current_context" != admin@tuppr-e2e-* ]]; then
        log "ERROR: Current context '$current_context' doesn't match expected pattern 'admin@tuppr-e2e-*'"
        log "       Refusing to proceed to avoid touching non-e2e clusters"
        exit 1
    fi

    # Verify node names match our cluster
    local node_names
    node_names=$(kubectl get nodes -o json | jq -r '.items[].metadata.name' | head -1)
    if [[ "$node_names" != "$EXPECTED_CLUSTER"* ]]; then
        log "ERROR: Node names don't start with '$EXPECTED_CLUSTER'"
        log "       Found: $node_names"
        log "       Refusing to proceed to avoid touching non-e2e clusters"
        exit 1
    fi

    log "Connected to cluster: $EXPECTED_CLUSTER"
    log "Target Talos version: $TALOS_TO_VERSION"
    log "Target Kubernetes version: $K8S_TO_VERSION"

    cd "$REPO_ROOT"

    if [[ -n "${CONTROLLER_IMAGE:-}" ]]; then
        log "Using pre-built controller image: $CONTROLLER_IMAGE"
    else
        log "Building controller image..."
        RUN_ID=$(date +%s)
        CONTROLLER_IMAGE="ttl.sh/tuppr-e2e-${RUN_ID}:2h"

        docker build -t "$CONTROLLER_IMAGE" .
        log "Pushing controller image..."
        docker push "$CONTROLLER_IMAGE"
        log "Controller image: $CONTROLLER_IMAGE"
    fi

    log "Waiting for cluster health checks..."
    NODE_IP=$(kubectl get nodes -o json | jq -r '.items[0].status.addresses[] | select(.type=="ExternalIP") | .address')
    talosctl --nodes $NODE_IP health --wait-timeout=10m

    log "Generating CRDs..."
    make helm-crds

    log "Creating tuppr-system namespace..."
    kubectl create namespace tuppr-system --dry-run=client -o yaml | kubectl apply -f -
    kubectl label namespace tuppr-system \
        pod-security.kubernetes.io/enforce=privileged \
        pod-security.kubernetes.io/warn=privileged \
        --overwrite

    log "Installing tuppr via Helm..."
    IFS=':' read -r REPO TAG <<< "$CONTROLLER_IMAGE"
    helm upgrade --install tuppr "${REPO_ROOT}/charts/tuppr" \
        --namespace tuppr-system \
        --set image.repository="$REPO" \
        --set image.tag="$TAG" \
        --set image.pullPolicy=Always \
        --set replicaCount=2 \
        --wait \
        --timeout 5m

    log "Starting background monitoring..."
    stern -n tuppr-system tuppr --since 1s 2>&1 | sed 's/^/[stern] /' &
    STERN_PID=$!

    kubectl get talosupgrade -w -o yaml 2>&1 | sed 's/^/[watch-talos] /' &
    WATCH_TALOS_PID=$!

    kubectl get kubernetesupgrade -w -o yaml 2>&1 | sed 's/^/[watch-k8s] /' &
    WATCH_K8S_PID=$!

    trap "kill $STERN_PID $WATCH_TALOS_PID $WATCH_K8S_PID 2>/dev/null || true" EXIT

    log "============================================"
    log "Starting TalosUpgrade test"
    log "============================================"

    log "Creating TalosUpgrade CR..."
    export TALOS_TO_VERSION
    envsubst < "${CR_TEMPLATES_DIR}/talos-upgrade.yaml" | kubectl apply -f -

    wait_for_cr_completed talosupgrade e2e-talos-upgrade 1200
    verify_talos_version "$TALOS_TO_VERSION"

    log "============================================"
    log "TalosUpgrade test PASSED"
    log "============================================"

    log "============================================"
    log "Starting KubernetesUpgrade test"
    log "============================================"

    log "Creating KubernetesUpgrade CR..."
    export K8S_TO_VERSION
    envsubst < "${CR_TEMPLATES_DIR}/k8s-upgrade.yaml" | kubectl apply -f -

    wait_for_cr_completed kubernetesupgrade e2e-k8s-upgrade 900
    verify_k8s_version "$K8S_TO_VERSION"

    log "============================================"
    log "KubernetesUpgrade test PASSED"
    log "============================================"

    log "Stopping background monitoring..."
    kill $STERN_PID $WATCH_TALOS_PID $WATCH_K8S_PID 2>/dev/null || true

    log ""
    log "=========================================="
    log "ALL E2E TESTS PASSED"
    log "=========================================="
}

main "$@"
