terraform {
  required_version = ">= 1.8"

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
  }
}

provider "hcloud" {
  token = var.hcloud_token
}
