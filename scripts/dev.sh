#!/usr/bin/env bash
# Full dev e2e — runs everything directly without going through uds run.
# This avoids maru timeout/env issues for long-running steps.
#
# Usage:
#   ./scripts/dev.sh                       # full e2e from scratch
#   ./scripts/dev.sh SKIP_IMAGES=1         # skip packer builds
#   ./scripts/dev.sh SKIP_WIPE=1           # keep existing k3s
#   ./scripts/dev.sh SKIP_IMAGES=1 SKIP_WIPE=1
#
# Also accepts maru-style: ./scripts/dev.sh --with SKIP_IMAGES=1
set -euo pipefail

cd "$(dirname "$0")/.."

# ── Parse args ────────────────────────────────────────────────────────────────
SKIP_IMAGES=0
SKIP_WIPE=0
SKIP_BASE=0
SKIP_TOOLS=0
SKIP_UDS_CORE=0

parse_kv() {
  local key="${1%%=*}" val="${1#*=}"
  case "$key" in
    SKIP_IMAGES)   SKIP_IMAGES="$val" ;;
    SKIP_WIPE)     SKIP_WIPE="$val" ;;
    SKIP_BASE)     SKIP_BASE="$val" ;;
    SKIP_TOOLS)    SKIP_TOOLS="$val" ;;
    SKIP_UDS_CORE) SKIP_UDS_CORE="$val" ;;
  esac
}

while [ $# -gt 0 ]; do
  case "$1" in
    --with) shift; parse_kv "$1" ;;
    *=*)    parse_kv "$1" ;;
  esac
  shift
done

# ── Sudo keepalive ────────────────────────────────────────────────────────────
if ! sudo -n true 2>/dev/null; then
  echo "This workflow requires sudo for k3s installation."
  sudo -v
fi
sudo_keepalive() {
  while true; do sleep 240; sudo -n true 2>/dev/null || break; done
}
sudo_keepalive &
KEEPALIVE_PID=$!
trap 'kill "$KEEPALIVE_PID" 2>/dev/null; wait "$KEEPALIVE_PID" 2>/dev/null || true' EXIT

# ── Packer SSH key ────────────────────────────────────────────────────────────
if [ ! -f packer/packer-key ]; then
  echo "Generating packer SSH keypair..."
  ssh-keygen -t ed25519 -f packer/packer-key -N "" -q
fi

# ── Build packer images ───────────────────────────────────────────────────────
if [ "$SKIP_IMAGES" = "1" ]; then
  echo "▶ Skipping packer builds (SKIP_IMAGES=1)"
else
  echo "▶ Building packer images — streaming to /tmp/uds-dev-packer.log"
  (cd packer && \
    SKIP_BASE="$SKIP_BASE" \
    SKIP_TOOLS="$SKIP_TOOLS" \
    SKIP_UDS_CORE="$SKIP_UDS_CORE" \
    ./build-images-qemu.sh) 2>&1 | tee /tmp/uds-dev-packer.log
fi

# ── Cluster setup + deploy + golden PVCs ─────────────────────────────────────
# Import golden PVCs only for qcow2s that actually exist.
# dev-cluster-setup.sh will skip missing tiers rather than die.
SKIP_GOLDEN_PVC=0
if [ "$SKIP_IMAGES" = "1" ] && \
   [ ! -f packer/output/base/lab-base.qcow2 ] && \
   [ ! -f packer/output/tools/lab-playground-tools.qcow2 ] && \
   [ ! -f packer/output/uds-core/lab-playground-uds-core.qcow2 ]; then
  echo "▶ No qcow2s found — skipping golden PVC import"
  SKIP_GOLDEN_PVC=1
fi

echo "▶ Setting up cluster — streaming to /tmp/uds-dev-cluster.log"
SKIP_WIPE="$SKIP_WIPE" \
SKIP_GOLDEN_PVC="$SKIP_GOLDEN_PVC" \
BASE_QCOW2=packer/output/base/lab-base.qcow2 \
TOOLS_QCOW2=packer/output/tools/lab-playground-tools.qcow2 \
UDS_CORE_QCOW2=packer/output/uds-core/lab-playground-uds-core.qcow2 \
  ./scripts/dev-cluster-setup.sh 2>&1 | tee /tmp/uds-dev-cluster.log

# ── CoreDNS patch ─────────────────────────────────────────────────────────────
echo "▶ Patching CoreDNS..."
./scripts/patch-coredns.sh

# ── Keycloak test user ────────────────────────────────────────────────────────
echo "▶ Creating Keycloak test user..."
uds run setup:keycloak-user --with group="/UDS Core/Admin"

# ── nginx proxy ───────────────────────────────────────────────────────────────
echo "▶ Starting nginx proxy..."
./scripts/start-proxy.sh

echo ""
echo "╔══════════════════════════════════════════════════════════╗"
echo "  UDS Lab Platform — dev environment ready"
echo ""
echo "  UI:    https://lab.uds.dev"
echo "  Admin: https://keycloak.admin.uds.dev"
echo ""
echo "  Test user: doug / unicorn123!@#UN"
echo "  Group:     /UDS Core/Admin"
echo "╚══════════════════════════════════════════════════════════╝"
