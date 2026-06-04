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

variable "branch_name" {
  description = "Git branch name for resource tracking and cleanup"
  type        = string
  default     = ""
}

variable "control_plane_count" {
  description = "Number of control plane nodes"
  type        = number
  default     = 3
}

variable "worker_count" {
  description = "Number of worker nodes"
  type        = number
  default     = 0
}

variable "talos_schematic_id" {
  description = "Talos Image Factory schematic ID for the hcloud installer/image"
  type        = string
  default     = "cc1be48f01f4bf4367226c4a20eeaf4651f62fc64cfe17f8576ede9fbe14a336"
}

variable "talos_bootstrap_version" {
  description = "Initial Talos Linux version to deploy"
  type        = string
  default     = "v1.12.4"
}

variable "location" {
  description = "Hetzner datacenter location"
  type        = string
  default     = "nbg1"
}

locals {
  run_id       = var.run_id != "" ? var.run_id : "local"
  cluster_name = "tuppr-e2e-${local.run_id}-${var.control_plane_count}cp-${var.worker_count}w"

  common_labels = {
    cluster             = local.cluster_name
    managed-by          = "tuppr-e2e"
    run-id              = local.run_id
    branch              = replace(var.branch_name, "/", "-")
    control-plane-count = tostring(var.control_plane_count)
    worker-count        = tostring(var.worker_count)
  }
}
