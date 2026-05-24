terraform {
  required_providers {
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "~> 1.49"
    }
  }
}

provider "hcloud" {}

locals {
  access_url = var.coder_access_url != "" ? var.coder_access_url : "http://${hcloud_server.coder.ipv4_address}:3333"
}

resource "hcloud_firewall" "coder" {
  name = "coder-server"

  rule {
    direction = "in"
    protocol  = "tcp"
    port      = "22"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  rule {
    direction = "in"
    protocol  = "tcp"
    port      = "3333"
    source_ips = ["0.0.0.0/0", "::/0"]
  }
}

resource "hcloud_server" "coder" {
  name         = "coder-server"
  server_type  = var.server_type
  image        = "ubuntu-24.04"
  location     = var.location
  firewall_ids = [hcloud_firewall.coder.id]
  ssh_keys     = var.ssh_key_names

  user_data = templatefile("${path.module}/user-data.sh.tftpl", {
    coder_version     = var.coder_version
    postgres_password = var.postgres_password
    access_url        = var.coder_access_url
  })

  labels = {
    managed_by = "atmos"
    component  = "coder-server"
  }
}
