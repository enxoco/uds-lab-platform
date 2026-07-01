packer {
  required_plugins {
    qemu = {
      version = ">= 1.0.0"
      source  = "github.com/hashicorp/qemu"
    }
  }
}

# Ubuntu 24.04 cloud image — disk_image=true treats iso_url as the boot disk,
# not a bootable ISO. cloud-init via cidata CD-ROM sets up the packer user.
source "qemu" "lab-base" {
  iso_url      = "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"
  iso_checksum = "file:https://cloud-images.ubuntu.com/noble/current/SHA256SUMS"
  disk_image   = true

  format           = "qcow2"
  output_directory = "output/base"
  vm_name          = "lab-base.qcow2"

  # Expand disk to 20G; cloud-init growpart fills it.
  disk_size = "20G"
  cpus      = 4
  memory    = 4096

  # Let packer manage netdev + SSH port-forward + CD-ROM attachment.
  # Do NOT use qemuargs here — it replaces packer's netdev and breaks SSH.
  accelerator  = "kvm"
  machine_type = "q35"
  cpu_model    = "host"
  net_device   = "virtio-net-pci"

  # cidata CD-ROM provides the NoCloud cloud-init datasource.
  cd_files = ["./cloud-init/user-data", "./cloud-init/meta-data"]
  cd_label = "cidata"

  communicator          = "ssh"
  ssh_username          = "packer"
  ssh_private_key_file  = "./packer-key"
  ssh_timeout           = "10m"

  vnc_password     = "packer"
  headless         = true
  shutdown_command = "sudo shutdown -P now"
  shutdown_timeout = "5m"
}

build {
  name    = "lab-base"
  sources = ["source.qemu.lab-base"]

  provisioner "shell" {
    script = "scripts/base.sh"
    execute_command = "sudo bash '{{ .Path }}'"
  }

  post-processor "manifest" {
    output     = "output/manifest-base.json"
    strip_path = true
  }
}
