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

variable "tools_image" {
  type        = string
  description = "uds-lab-playground-tools snapshot name or ID"
  default     = env("TOOLS_IMAGE")
}

variable "location" {
  type    = string
  default = "hil"
}

source "hcloud" "playground-uds-core" {
  token         = var.hcloud_token
  image         = var.tools_image
  location      = var.location
  server_type   = "ccx13"
  snapshot_name = "uds-lab-playground-uds-core-${formatdate("YYYYMMDD-hhmmss", timestamp())}"
  snapshot_labels = {
    role = "uds-lab-playground"
    tier = "uds-core"
  }
  ssh_username = "root"
}

build {
  sources = ["source.hcloud.playground-uds-core"]

  provisioner "shell" {
    script = "scripts/playground-uds-core.sh"
  }

  post-processor "manifest" {
    output     = "manifest-playground-uds-core.json"
    strip_path = true
  }
}
