packer {
  required_plugins {
    hcloud = {
      version = ">= 1.4.0"
      source  = "github.com/hetznercloud/hcloud"
    }
  }
}

variable "hcloud_token" {
  type      = string
  sensitive = true
  default   = env("HCLOUD_TOKEN")
}

variable "location" {
  type    = string
  default = "hil"
}

source "hcloud" "lab-base" {
  token         = var.hcloud_token
  image         = "ubuntu-24.04"
  location      = var.location
  server_type   = "cpx11"
  snapshot_name = "uds-lab-base-${formatdate("YYYYMMDD-hhmmss", timestamp())}"
  snapshot_labels = {
    role = "uds-lab-base"
  }
  ssh_username = "root"
}

build {
  sources = ["source.hcloud.lab-base"]

  provisioner "shell" {
    script = "scripts/base.sh"
  }

  post-processor "manifest" {
    output     = "manifest.json"
    strip_path = true
  }
}
