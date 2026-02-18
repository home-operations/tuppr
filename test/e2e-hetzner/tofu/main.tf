module "talos_cluster" {
  source  = "hcloud-talos/talos/hcloud"
  version = "3.1.1"

  cluster_name   = local.cluster_name
  cluster_prefix = true

  talos_version      = var.talos_bootstrap_version
  kubernetes_version = var.k8s_bootstrap_version

  talos_iso_id_x86 = 122630
  disable_arm = true

  hcloud_token = var.hcloud_token

  location_name = var.location

  cluster_api_host           = hcloud_load_balancer.kube_api.ipv4
  kubeconfig_endpoint_mode   = "public_endpoint"
  talosconfig_endpoints_mode = "public_ip"

  firewall_kube_api_source  = ["0.0.0.0/0"]
  firewall_talos_api_source = ["0.0.0.0/0"]

  enable_floating_ip = false

  control_plane_nodes = [
    for i in range(var.control_plane_count) : {
      id   = i + 1
      type = var.server_type
    }
  ]

  worker_nodes = [
    for i in range(var.worker_count) : {
      id   = i + 1
      type = var.server_type
    }
  ]

  talos_control_plane_extra_config_patches = [
    yamlencode({
      machine = {
        features = {
          kubernetesTalosAPIAccess = {
            enabled                     = true
            allowedRoles                = ["os:admin"]
            allowedKubernetesNamespaces = ["tuppr-system"]
          }
        }
      }
      cluster = {
        controlPlane = {
          endpoint = "https://${hcloud_load_balancer.kube_api.ipv4}:6443"
        }
      }
    })
  ]
}

resource "local_file" "kubeconfig" {
  content         = module.talos_cluster.kubeconfig
  filename        = "${local.config_dir}/kubeconfig"
  file_permission = "0600"
}

resource "local_file" "talosconfig" {
  content         = module.talos_cluster.talosconfig
  filename        = "${local.config_dir}/talosconfig"
  file_permission = "0600"
}
