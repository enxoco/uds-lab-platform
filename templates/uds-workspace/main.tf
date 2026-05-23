terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = "~> 2.0"
    }
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "~> 1.49"
    }
  }
}

provider "hcloud" {
  token = var.hetzner_token
}

data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

data "coder_parameter" "setup_mode" {
  name         = "setup_mode"
  display_name = "Setup Mode"
  description  = "Which UDS stack to bootstrap in this workspace"
  type         = "string"
  default      = "uds-core-slim"
  mutable      = false

  option {
    name        = "UDS Core Slim Dev"
    value       = "uds-core-slim"
    description = "uds deploy k3d-core-slim-dev:latest — k3d cluster + Istio + Keycloak"
  }
  option {
    name        = "Zarf Init"
    value       = "zarf-init"
    description = "K3s + zarf init"
  }
}

resource "coder_agent" "main" {
  arch           = "amd64"
  os             = "linux"
  startup_script = file("${path.module}/scripts/startup.sh")

  env = {
    SETUP_MODE = data.coder_parameter.setup_mode.value
  }

  metadata {
    display_name = "K3s"
    key          = "k3s_status"
    script       = "systemctl is-active k3s 2>/dev/null || echo inactive"
    interval     = 10
    timeout      = 5
  }

  metadata {
    display_name = "CPU"
    key          = "cpu_usage"
    script       = "top -bn1 | grep 'Cpu(s)' | awk '{print $2}' | cut -d. -f1"
    interval     = 10
    timeout      = 5
  }

  metadata {
    display_name = "RAM"
    key          = "ram_usage"
    script       = "free -h | awk '/^Mem/{print $3\"/\"$2}'"
    interval     = 10
    timeout      = 5
  }

  metadata {
    display_name = "Disk"
    key          = "disk_usage"
    script       = "df -h / | awk 'NR==2{print $3\"/\"$2}'"
    interval     = 30
    timeout      = 5
  }
}

resource "hcloud_server" "workspace" {
  count       = data.coder_workspace.me.start_count
  name        = "coder-${lower(data.coder_workspace_owner.me.name)}-${lower(data.coder_workspace.me.name)}"
  server_type = var.server_type
  image       = "ubuntu-24.04"
  location    = var.location

  user_data = templatefile("${path.module}/scripts/user-data.sh.tftpl", {
    init_script       = coder_agent.main.init_script
    coder_agent_token = coder_agent.main.token
  })

  labels = {
    coder_workspace = data.coder_workspace.me.id
    owner           = data.coder_workspace_owner.me.name
  }
}

resource "coder_metadata" "workspace" {
  count       = data.coder_workspace.me.start_count
  resource_id = hcloud_server.workspace[0].id

  item {
    key   = "server_type"
    value = var.server_type
  }
  item {
    key   = "location"
    value = var.location
  }
  item {
    key   = "ipv4"
    value = hcloud_server.workspace[0].ipv4_address
  }
  item {
    key   = "setup_mode"
    value = data.coder_parameter.setup_mode.value
  }
}

resource "coder_app" "terminal" {
  agent_id     = coder_agent.main.id
  slug         = "terminal"
  display_name = "Terminal"
  icon         = "/icon/terminal.svg"
  command      = "bash"
}
