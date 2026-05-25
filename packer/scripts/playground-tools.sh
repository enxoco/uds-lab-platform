#!/bin/bash
# Builds the tools playground image. Installs Docker, k3d, and uds CLI
# on top of the uds-lab-base image.
set -euo pipefail

export HOME=/root
export DEBIAN_FRONTEND=noninteractive

log() { echo "[$(date '+%H:%M:%S')] $*"; }

# ── Dev tools ─────────────────────────────────────────────────────────────────
log "Installing dev tools..."
apt-get update -q
apt-get install -y -q neovim jq yamllint

YQ_VERSION=$(curl -s https://api.github.com/repos/mikefarah/yq/releases/latest \
  | grep '"tag_name"' | cut -d'"' -f4)
curl -sSL \
  "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_amd64" \
  -o /usr/local/bin/yq
chmod +x /usr/local/bin/yq
log "yq: $(yq --version)"

# ── Docker ─────────────────────────────────────────────────────────────────────
log "Installing Docker..."
curl -fsSL https://get.docker.com | sh
systemctl enable docker
systemctl start docker
log "Docker: $(docker --version)"

# ── k3d ───────────────────────────────────────────────────────────────────────
log "Installing k3d..."
curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash
log "k3d: $(k3d version)"

# ── uds CLI ───────────────────────────────────────────────────────────────────
log "Installing uds CLI..."
UDS_VERSION=$(curl -s https://api.github.com/repos/defenseunicorns/uds-cli/releases/latest \
  | grep '"tag_name"' | cut -d'"' -f4)
curl -sSL \
  "https://github.com/defenseunicorns/uds-cli/releases/download/${UDS_VERSION}/uds-cli_${UDS_VERSION}_Linux_amd64" \
  -o /usr/local/bin/uds
chmod +x /usr/local/bin/uds
log "uds: $(uds version)"

# ── Clean up ───────────────────────────────────────────────────────────────────
log "Cleaning up..."
apt-get clean
rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*
log "Tools playground build complete."
