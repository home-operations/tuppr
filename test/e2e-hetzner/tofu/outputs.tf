output "kubeconfig_path" {
  description = "Path to generated kubeconfig file"
  value       = local_file.kubeconfig.filename
}

output "talosconfig_path" {
  description = "Path to generated talosconfig file"
  value       = local_file.talosconfig.filename
}

output "talos_upgrade_version" {
  description = "Target Talos version for upgrade testing"
  value       = var.talos_upgrade_version
}

output "k8s_upgrade_version" {
  description = "Target Kubernetes version for upgrade testing"
  value       = var.k8s_upgrade_version
}

output "cluster_name" {
  description = "Generated cluster name"
  value       = local.cluster_name
}

output "control_plane_ips" {
  description = "Control plane node IP addresses"
  value       = module.talos_cluster.public_ipv4_list
}

output "load_balancer_ip" {
  description = "Load balancer IP address"
  value       = hcloud_load_balancer.kube_api.ipv4
}
