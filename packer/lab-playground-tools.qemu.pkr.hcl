packer {
  required_plugins {
    qemu = {
      version = ">= 1.0.0"
      source  = "github.com/hashicorp/qemu"
    }
  }
}

variable "base_image" {
  type        = string
  description = "Path to lab-base.qcow2 produced by lab-base.qemu.pkr.hcl"
  default     = "output/base/lab-base.qcow2"
}

source "qemu" "lab-playground-tools" {
  iso_url      = var.base_image
  iso_checksum = "none"
  disk_image   = true

  format           = "qcow2"
  output_directory = "output/tools"
  vm_name          = "lab-playground-tools.qcow2"

  # Docker images add bulk; grow to 50G.
  disk_size = "50G"
  cpus      = 4
  memory    = 4096

  accelerator  = "kvm"
  machine_type = "q35"
  cpu_model    = "host"
  net_device   = "virtio-net-pci"

  # cloud-init is disabled in the base image after first boot; no cidata needed.
  communicator         = "ssh"
  ssh_username         = "packer"
  ssh_private_key_file = "./packer-key"
  ssh_timeout          = "15m"

  vnc_password     = "packer"
  headless         = true
  shutdown_command = "sudo shutdown -P now"
  shutdown_timeout = "5m"
}

build {
  name    = "lab-playground-tools"
  sources = ["source.qemu.lab-playground-tools"]

  provisioner "shell" {
    script          = "scripts/playground-tools.sh"
    execute_command = "sudo bash '{{ .Path }}'"
  }

  post-processor "manifest" {
    output     = "output/manifest-tools.json"
    strip_path = true
  }
}
