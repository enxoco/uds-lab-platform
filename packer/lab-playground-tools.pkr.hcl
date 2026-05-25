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

variable "base_image" {
  type        = string
  description = "uds-lab-base snapshot name or ID"
  default     = env("BASE_IMAGE")
}

variable "location" {
  type    = string
  default = "hil"
}

source "hcloud" "playground-tools" {
  token         = var.hcloud_token
  image         = var.base_image
  location      = var.location
  server_type   = "ccx13"
  snapshot_name = "uds-lab-playground-tools-${formatdate("YYYYMMDD-hhmmss", timestamp())}"
  snapshot_labels = {
    role = "uds-lab-playground"
    tier = "tools"
  }
  ssh_username = "root"
}

build {
  sources = ["source.hcloud.playground-tools"]

  provisioner "shell" {
    script = "scripts/playground-tools.sh"
  }

  post-processor "manifest" {
    output     = "manifest-playground-tools.json"
    strip_path = true
  }
}
