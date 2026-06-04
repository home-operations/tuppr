resource "imager_image" "talos_x86" {
  image_url    = "https://factory.talos.dev/image/${var.talos_schematic_id}/${var.talos_bootstrap_version}/hcloud-amd64.raw.xz"
  architecture = "x86"
  location     = var.location

  description = "Talos ${var.talos_bootstrap_version} (tuppr-e2e)"

  labels = merge(local.common_labels, {
    talos-version = var.talos_bootstrap_version
    schematic     = substr(var.talos_schematic_id, 0, 12)
  })
}
