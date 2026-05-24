#!/bin/bash
set -euo pipefail

# renovate: datasource=github-releases depName=defenseunicorns/uds-cli
UDS_VERSION="0.31.0"
# renovate: datasource=github-releases depName=zarf-dev/zarf
ZARF_VERSION="0.76.0"
SETUP_MODE="${SETUP_MODE:-uds-core-slim}"

log() { echo "[startup] $*"; }

install_docker() {
  if command -v docker &>/dev/null; then
    log "docker already installed"
    return
  fi
  log "installing docker..."
  curl -fsSL https://get.docker.com | sh
  systemctl enable --now docker
}

install_uds_cli() {
  if command -v uds &>/dev/null; then
    log "uds already installed"
    return
  fi
  log "installing uds v${UDS_VERSION}..."
  curl -fsSL -o /usr/local/bin/uds \
    "https://github.com/defenseunicorns/uds-cli/releases/download/v${UDS_VERSION}/uds-cli_v${UDS_VERSION}_Linux_amd64"
  chmod +x /usr/local/bin/uds
}

install_zarf() {
  if command -v zarf &>/dev/null; then
    log "zarf already installed"
    return
  fi
  log "installing zarf v${ZARF_VERSION}..."
  curl -fsSL -o /usr/local/bin/zarf \
    "https://github.com/zarf-dev/zarf/releases/download/v${ZARF_VERSION}/zarf_v${ZARF_VERSION}_Linux_amd64"
  chmod +x /usr/local/bin/zarf
}

install_k3d() {
  if command -v k3d &>/dev/null; then
    log "k3d already installed"
    return
  fi
  log "installing k3d..."
  curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash
}

configure_kubeconfig() {
  mkdir -p "${HOME}/.kube"
  k3d kubeconfig merge --all -o "${HOME}/.kube/config"
  chmod 600 "${HOME}/.kube/config"
  export KUBECONFIG="${HOME}/.kube/config"
}

mode_uds_core_slim() {
  log "mode: uds deploy k3d-core-slim-dev:latest"
  install_docker
  install_k3d
  install_uds_cli
  uds deploy k3d-core-slim-dev:latest --confirm
}

mode_zarf_init() {
  log "mode: zarf init on k3d"
  install_docker
  install_k3d
  install_zarf
  k3d cluster create zarf --wait
  configure_kubeconfig
  zarf init --confirm
}

case "${SETUP_MODE}" in
  uds-core-slim) mode_uds_core_slim ;;
  zarf-init)     mode_zarf_init ;;
  *) log "unknown SETUP_MODE: ${SETUP_MODE}"; exit 1 ;;
esac

log "workspace ready"
