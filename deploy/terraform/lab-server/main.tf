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
  access_url = var.domain != "" ? "https://${var.domain}" : "http://${hcloud_server.lab_server.ipv4_address}"
}

resource "hcloud_firewall" "lab_server" {
  name = "lab-server"

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "22"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "80"
    source_ips = ["0.0.0.0/0", "::/0"]
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "443"
    source_ips = ["0.0.0.0/0", "::/0"]
  }
}

resource "hcloud_server" "lab_server" {
  name         = "lab-server"
  server_type  = var.server_type
  image        = "ubuntu-24.04"
  location     = var.location
  firewall_ids = [hcloud_firewall.lab_server.id]
  ssh_keys     = var.ssh_key_names

  user_data = templatefile("${path.module}/user-data.sh.tftpl", {
    domain         = var.domain
    session_ttl    = var.session_ttl
    hcloud_token   = var.hcloud_token_for_vms
  })

  labels = {
    managed_by = "tofu"
    component  = "lab-server"
  }
}
