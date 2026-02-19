resource "hcloud_load_balancer" "kube_api" {
  name               = "${local.cluster_name}-lb"
  load_balancer_type = "lb11"
  location           = var.location
  labels             = local.common_labels
}

resource "hcloud_load_balancer_service" "kube_api" {
  load_balancer_id = hcloud_load_balancer.kube_api.id
  protocol         = "tcp"
  listen_port      = 6443
  destination_port = 6443

  health_check {
    protocol = "tcp"
    port     = 6443
    interval = 10
    timeout  = 5
    retries  = 3
  }
}

resource "hcloud_load_balancer_target" "control_planes" {
  type             = "label_selector"
  load_balancer_id = hcloud_load_balancer.kube_api.id
  label_selector   = "cluster=${local.cluster_name},role=control-plane"
  use_private_ip   = false
}
