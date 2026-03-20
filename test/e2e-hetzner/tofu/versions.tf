terraform {
  required_version = ">= 1.9"

  required_providers {
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "~> 1.60"
    }
    talos = {
      source  = "siderolabs/talos"
      version = "~> 0.10"
    }
    local = {
      source  = "hashicorp/local"
      version = "~> 2.6"
    }
    imager = {
      source  = "hcloud-talos/imager"
      version = "~> 0.1"
    }
  }
}

provider "hcloud" {
  token = var.hcloud_token
}

provider "imager" {
  token = var.hcloud_token
}
