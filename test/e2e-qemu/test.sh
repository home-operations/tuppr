#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CR_TEMPLATES_DIR="${SCRIPT_DIR}/cr-templates"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
# shellcheck source=common.sh
source "${SCRIPT_DIR}/common.sh"

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

node_ips() {
    kubectl get nodes -o json |
        jq -r '[.items[].status.addresses[] | select(.type=="InternalIP") | .address] | join(",")'
}

verify_talos_version() {
    local expected_version=$1
    log "Verifying all nodes are at Talos version $expected_version..."

    local output
    output=$(talosctl version --short --nodes "$(node_ips)" 2>&1)

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

# Apply a rendered CR template, tolerating the brief window right after the Helm
# install where the webhook's Service has no ready endpoints yet. The webhook is
# failurePolicy: Fail, and /readyz gates on the webhook server starting, so
# `helm --wait` returns once the pod is Ready. Programming that pod into the
# Service's endpoints lags Ready by a moment, and until kube-proxy has the route
# the API server rejects the create with "connection refused". A GitOps tool
# would just retry, so the test does too rather than reading the race as failure.
apply_cr() {
    local template=$1
    local rendered deadline err
    rendered=$(envsubst < "$template")
    deadline=$((SECONDS + 120))

    while true; do
        if err=$(kubectl apply -f - <<<"$rendered" 2>&1); then
            echo "$err"
            return 0
        fi
        if [[ "$err" != *webhook* ]] || [[ $SECONDS -ge $deadline ]]; then
            echo "$err" >&2
            return 1
        fi
        log "Webhook not reachable yet, retrying CR apply..."
        sleep 5
    done
}

run_talos_upgrade() {
    log "============================================"
    log "Starting TalosUpgrade test"
    log "============================================"

    log "Creating TalosUpgrade CR..."
    export TALOS_UPGRADE_VERSION
    apply_cr "${CR_TEMPLATES_DIR}/talos-upgrade.yaml"

    wait_for_cr_completed talosupgrade e2e-talos-upgrade 1200
    verify_talos_version "$TALOS_UPGRADE_VERSION"

    log "============================================"
    log "TalosUpgrade test PASSED"
    log "============================================"
}

run_kubernetes_upgrade() {
    log "============================================"
    log "Starting KubernetesUpgrade test"
    log "============================================"

    log "Creating KubernetesUpgrade CR..."
    export K8S_UPGRADE_VERSION
    apply_cr "${CR_TEMPLATES_DIR}/k8s-upgrade.yaml"

    wait_for_cr_completed kubernetesupgrade e2e-k8s-upgrade 900
    verify_k8s_version "$K8S_UPGRADE_VERSION"

    log "============================================"
    log "KubernetesUpgrade test PASSED"
    log "============================================"
}

main() {
    # Which upgrade this leg drives. One kind per leg: nothing tests a Kubernetes
    # upgrade on top of a Talos one, so a failure names the controller that broke.
    case "${UPGRADE_KIND:-}" in
        talos | kubernetes) ;;
        *)
            log "ERROR: UPGRADE_KIND must be 'talos' or 'kubernetes', got '${UPGRADE_KIND:-}'"
            exit 1
            ;;
    esac

    # talosctl-cluster-action exports the two configs and reports the cluster name;
    # a local run points them at whatever cluster it built itself.
    : "${KUBECONFIG:?must point at the e2e cluster kubeconfig}"
    : "${TALOSCONFIG:?must point at the e2e cluster talosconfig}"
    : "${CLUSTER_NAME:?must be metadata.name from the leg document}"

    # Where the Talos API calls go. CI passes the action's endpoint output; the
    # talosconfig names the same address.
    if [[ -z "${ENDPOINT:-}" ]]; then
        ENDPOINT=$(talosctl config info -o json | jq -r '.endpoints[0] // empty')
    fi
    : "${ENDPOINT:?no control plane address in $TALOSCONFIG}"

    log "Kubeconfig: $KUBECONFIG"
    log "Talosconfig: $TALOSCONFIG"

    # Refuse to touch anything that is not one of our throwaway clusters. Talos
    # derives the kubectl context and every node name from the cluster name, so
    # both have to agree with it.
    if [[ "$CLUSTER_NAME" != tuppr-e2e-* ]]; then
        log "ERROR: Cluster name '$CLUSTER_NAME' doesn't match expected pattern 'tuppr-e2e-*'"
        log "       Refusing to proceed to avoid touching non-e2e clusters"
        exit 1
    fi

    local current_context
    current_context=$(kubectl config current-context)
    if [[ "$current_context" != "admin@${CLUSTER_NAME}" ]]; then
        log "ERROR: Current context '$current_context' is not 'admin@${CLUSTER_NAME}'"
        log "       Refusing to proceed to avoid touching non-e2e clusters"
        exit 1
    fi

    local node_names
    node_names=$(kubectl get nodes -o json | jq -r '.items[].metadata.name' | head -1)
    if [[ "$node_names" != "$CLUSTER_NAME"* ]]; then
        log "ERROR: Node names don't start with '$CLUSTER_NAME'"
        log "       Found: $node_names"
        log "       Refusing to proceed to avoid touching non-e2e clusters"
        exit 1
    fi

    log "Connected to cluster: $CLUSTER_NAME"
    log "Target Talos version: $TALOS_UPGRADE_VERSION"
    log "Target Kubernetes version: $K8S_UPGRADE_VERSION"

    cd "$REPO_ROOT"

    # CI sets CONTROLLER_IMAGE and builds it concurrently with the cluster, so
    # by the time we get here there is nothing to do.
    if [[ -n "${CONTROLLER_IMAGE:-}" ]]; then
        log "Using pre-built controller image: $CONTROLLER_IMAGE"
    else
        CONTROLLER_IMAGE="ttl.sh/tuppr-e2e-$(date +%s):2h"
        export CONTROLLER_IMAGE
        "${SCRIPT_DIR}/image.sh"
    fi

    log "Waiting for cluster health checks..."
    talosctl --nodes "$ENDPOINT" health --wait-timeout=10m

    log "Generating CRDs..."
    mise run -C "$REPO_ROOT" helm-crds

    log "Creating tuppr-system namespace..."
    kubectl create namespace tuppr-system --dry-run=client -o yaml | kubectl apply -f -
    kubectl label namespace tuppr-system \
        pod-security.kubernetes.io/enforce=privileged \
        pod-security.kubernetes.io/warn=privileged \
        --overwrite

    log "Packaging Helm chart..."
    IFS=':' read -r REPO TAG <<< "$CONTROLLER_IMAGE"

    # A throwaway semver just for the OCI tag; the real version is stamped at
    # release time. The e2e only cares that the packaged artifact installs.
    local chart_version="0.0.0-e2e"
    local chart_dir chart_tgz chart_ref
    chart_dir=$(mktemp -d)
    helm package "${REPO_ROOT}/charts/tuppr" \
        --version "$chart_version" --app-version "$chart_version" --destination "$chart_dir"
    chart_tgz="${chart_dir}/tuppr-${chart_version}.tgz"

    # Install the chart the way it is actually distributed: pushed to an OCI
    # registry and pulled back, rather than from the working tree. CI points
    # CHART_REGISTRY at the runner-local registry; a local run has none, so it
    # installs the packaged tgz directly, which still exercises the artifact.
    local helm_oci_args=()
    if [[ -n "${CHART_REGISTRY:-}" ]]; then
        # --plain-http only for the loopback CI registry, which serves HTTP.
        case "$CHART_REGISTRY" in
            *localhost* | *127.0.0.1*) helm_oci_args=(--plain-http) ;;
        esac
        log "Pushing chart to ${CHART_REGISTRY} and installing from there..."
        helm push "$chart_tgz" "$CHART_REGISTRY" "${helm_oci_args[@]}"
        chart_ref="${CHART_REGISTRY}/tuppr"
        helm_oci_args+=(--version "$chart_version")
    else
        log "No CHART_REGISTRY set; installing the packaged chart directly."
        chart_ref="$chart_tgz"
    fi

    log "Installing tuppr via Helm..."
    helm upgrade --install tuppr "$chart_ref" "${helm_oci_args[@]}" \
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

    trap 'kill "$STERN_PID" "$WATCH_TALOS_PID" "$WATCH_K8S_PID" 2>/dev/null || true' EXIT

    case "$UPGRADE_KIND" in
        talos) run_talos_upgrade ;;
        kubernetes) run_kubernetes_upgrade ;;
    esac

    log "Stopping background monitoring..."
    kill $STERN_PID $WATCH_TALOS_PID $WATCH_K8S_PID 2>/dev/null || true

    log ""
    log "=========================================="
    log "E2E TESTS PASSED ($UPGRADE_KIND)"
    log "=========================================="
}

main "$@"
