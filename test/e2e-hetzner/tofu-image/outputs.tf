output "image_id" {
  description = "Hetzner Cloud image ID for the materialized Talos x86 snapshot"
  value       = imager_image.talos_x86.image_id
}
