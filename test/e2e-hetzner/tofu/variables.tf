variable "hcloud_token" {
  description = "Hetzner Cloud API token"
  type        = string
  sensitive   = true
}

variable "run_id" {
  description = "Unique run identifier for resource naming and tracking. Auto-generated if not provided."
  type        = string
  default     = ""
}

variable "control_plane_count" {
  description = "Number of control plane nodes"
  type        = number
  default     = 3

  validation {
    condition     = var.control_plane_count >= 1 && var.control_plane_count <= 10
    error_message = "Control plane count must be between 1 and 10"
  }
}

variable "worker_count" {
  description = "Number of worker nodes"
  type        = number
  default     = 0

  validation {
    condition     = var.worker_count >= 0 && var.worker_count <= 20
    error_message = "Worker count must be between 0 and 20"
  }
}

variable "talos_bootstrap_version" {
  description = "Initial Talos Linux version to deploy"
  type        = string
  default     = "v1.11.0"
}

variable "talos_upgrade_version" {
  description = "Target Talos Linux version for upgrade testing"
  type        = string
  default     = "v1.12.4"
}

variable "k8s_bootstrap_version" {
  description = "Initial Kubernetes version to deploy"
  type        = string
  default     = "v1.34.0"
}

variable "k8s_upgrade_version" {
  description = "Target Kubernetes version for upgrade testing"
  type        = string
  default     = "v1.35.0"
}

variable "server_type" {
  description = "Hetzner server type"
  type        = string
  default     = "cx23"
}

variable "location" {
  description = "Hetzner datacenter location"
  type        = string
  default     = "nbg1"
}

variable "config_dir" {
  description = "Directory for generated kubeconfig and talosconfig files. Auto-generated if not provided."
  type        = string
  default     = ""
}

locals {
  run_id       = var.run_id != "" ? var.run_id : "local"
  cluster_name = "tuppr-e2e-${local.run_id}-${var.control_plane_count}cp-${var.worker_count}w"
  config_dir   = var.config_dir != "" ? var.config_dir : "/tmp/tuppr-e2e-${local.run_id}"

  common_labels = {
    managed-by          = "tuppr-e2e"
    run-id              = local.run_id
    control-plane-count = tostring(var.control_plane_count)
    worker-count        = tostring(var.worker_count)
  }
}
