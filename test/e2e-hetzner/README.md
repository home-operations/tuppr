# Hetzner E2E Tests

End-to-end tests for tuppr controller on Hetzner Cloud using OpenTofu and Bash.

## Overview

This test suite provisions a Talos Kubernetes cluster on Hetzner Cloud, deploys the tuppr controller, and validates both Talos and Kubernetes upgrade functionality with HA maintained via load balancer.

## Quick Start

### 1. Set Environment Variables

**Required:**
```bash
export TF_VAR_hcloud_token="your-hetzner-token"
```

**Optional:**
```bash
export TF_VAR_run_id="custom-id"              # Default: "local"
export TF_VAR_control_plane_count=3           # Default: 3
export TF_VAR_worker_count=0                  # Default: 0
export TF_VAR_talos_bootstrap_version=v1.11.0 # Default: v1.11.0
export TF_VAR_k8s_bootstrap_version=v1.34.0   # Default: v1.34.0
export CONTROLLER_IMAGE="custom-image:tag"    # Default: builds from source
```

### 2. Provision Infrastructure

```bash
cd tofu
tofu init
tofu apply
```

This creates:
- Hetzner Cloud network
- Server instances
- Load balancer for Kubernetes API
- Talos cluster with Cilium CNI
- Kubeconfig and talosconfig in `/tmp/tuppr-e2e-{run_id}/`

**Cluster naming:** `tuppr-e2e-{run_id}-{cp}cp-{w}w`
- Example: `tuppr-e2e-local-3cp-0w` (local run, 3 control planes, 0 workers)
- Example: `tuppr-e2e-1234567-1cp-2w` (GHA run #1234567, 1 control plane, 2 workers)

### 3. Run Tests

```bash
cd ..
./test.sh
```

The test script:
1. Builds and pushes controller image (or uses `$CONTROLLER_IMAGE`)
2. Waits for nodes Ready
3. Installs tuppr with Helm
5. Tests TalosUpgrade
6. Tests KubernetesUpgrade

### 4. Cleanup

```bash
cd tofu
tofu destroy
```

## Variables Reference

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `hcloud_token` | string | (required) | Hetzner Cloud API token |
| `run_id` | string | `"local"` | Unique run identifier |
| `control_plane_count` | number | `3` | Number of control plane nodes (1-10) |
| `worker_count` | number | `0` | Number of worker nodes (0-20) |
| `talos_bootstrap_version` | string | `"v1.11.0"` | Initial Talos version |
| `talos_upgrade_version` | string | `"v1.12.4"` | Target Talos version |
| `k8s_bootstrap_version` | string | `"v1.34.0"` | Initial Kubernetes version |
| `k8s_upgrade_version` | string | `"v1.35.0"` | Target Kubernetes version |
| `server_type` | string | `"cx23"` | Hetzner server type |
| `location` | string | `"nbg1"` | Hetzner datacenter location |
| `config_dir` | string | `/tmp/tuppr-e2e-{run_id}` | Config file directory |

## Outputs

| Output | Description |
|--------|-------------|
| `kubeconfig_path` | Path to generated kubeconfig |
| `talosconfig_path` | Path to generated talosconfig |
| `talos_upgrade_version` | Target Talos version |
| `k8s_upgrade_version` | Target Kubernetes version |
| `cluster_name` | Generated cluster name |
| `control_plane_ips` | Control plane node IPs |
| `load_balancer_ip` | Load balancer IP |

