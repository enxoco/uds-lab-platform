packer {
  required_plugins {
    qemu = {
      version = ">= 1.0.0"
      source  = "github.com/hashicorp/qemu"
    }
  }
}

variable "tools_image" {
  type        = string
  description = "Path to lab-playground-tools.qcow2"
  default     = "output/tools/lab-playground-tools.qcow2"
}

source "qemu" "lab-playground-uds-core" {
  iso_url      = var.tools_image
  iso_checksum = "none"
  disk_image   = true

  format           = "qcow2"
  output_directory = "output/uds-core"
  vm_name          = "lab-playground-uds-core.qcow2"

  # UDS Core images (Istio, Keycloak, Prometheus, etc.) can be 20-30GB of
  # Docker layer storage on top of the tools base.
  disk_size = "80G"
  cpus      = 4
  memory    = 8192

  accelerator  = "kvm"
  machine_type = "q35"
  cpu_model    = "host"
  net_device   = "virtio-net-pci"

  communicator         = "ssh"
  ssh_username         = "packer"
  ssh_private_key_file = "./packer-key"
  ssh_timeout          = "30m"

  vnc_password     = "packer"
  headless         = true
  shutdown_command = "sudo shutdown -P now"
  # UDS Core deploy can take 10-15 min; allow generous shutdown timeout for
  # clean Docker/k3d teardown.
  shutdown_timeout = "10m"
}

build {
  name    = "lab-playground-uds-core"
  sources = ["source.qemu.lab-playground-uds-core"]

  provisioner "shell" {
    script          = "scripts/playground-uds-core.sh"
    execute_command = "sudo bash '{{ .Path }}'"
    # UDS Core deploy is slow; generous timeout.
    timeout = "40m"
  }

  post-processor "manifest" {
    output     = "output/manifest-uds-core.json"
    strip_path = true
  }
}
