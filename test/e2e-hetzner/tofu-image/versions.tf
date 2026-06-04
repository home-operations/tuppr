terraform {
  required_version = ">= 1.9"

  required_providers {
    imager = {
      source  = "hcloud-talos/imager"
      version = "~> 1.0"
    }
  }
}

provider "imager" {
  token = var.hcloud_token
}
