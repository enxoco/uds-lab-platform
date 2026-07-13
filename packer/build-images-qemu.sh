#!/usr/bin/env bash
# Build all lab qcow2 images in order: base → uds-core.
# The base image combines what were previously the separate base and tools stages.
# Each stage uses the previous stage's qcow2 as its input disk.
#
# Prerequisites:
#   - /dev/kvm available (AMD-V or Intel VT-x)
#   - packer >= 1.9, qemu-system-x86_64, qemu-img
#   - virt-customize (libguestfs-tools) — strips cloud-init.disabled after build
#   - 80+ GB free disk space in packer/output/
#   - Internet access (pulls Ubuntu cloud image + packages)
#
# Usage:
#   cd packer/
#   ./build-images-qemu.sh
#
#   # Skip stages already built:
#   SKIP_BASE=1 BASE_IMAGE=output/base/lab-base.qcow2 ./build-images-qemu.sh
#   SKIP_BASE=1 SKIP_UDS_CORE=1 ./build-images-qemu.sh  # no-op (all skipped)

set -euo pipefail
cd "$(dirname "$0")"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
log()  { echo -e "${GREEN}[$(date '+%H:%M:%S')]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
die()  { echo -e "${RED}[ERROR]${NC} $*" >&2; exit 1; }

# ── Preflight ─────────────────────────────────────────────────────────────────
ls /dev/kvm &>/dev/null                   || die "/dev/kvm not found — KVM required"
command -v packer             &>/dev/null || die "packer not found"
command -v qemu-img           &>/dev/null || die "qemu-img not found (install qemu-utils)"
command -v qemu-system-x86_64 &>/dev/null || die "qemu-system-x86_64 not found"
command -v virt-customize     &>/dev/null || die "virt-customize not found (Arch: pacman -S guestfs-tools | Debian/Ubuntu: apt install guestfs-tools)"

mkdir -p output

# Strip /etc/cloud/cloud-init.disabled so cloned VMs run cloud-init on first boot.
# The packer user-data now does 'cloud-init clean' instead of disabling it, but
# run this as belt-and-suspenders for any images built before that fix.
fix_cloud_init() {
  local img="$1"
  log "Stripping cloud-init.disabled from $img..."
  virt-customize -a "$img" --run-command 'rm -f /etc/cloud/cloud-init.disabled' \
    || warn "virt-customize failed on $img — manually verify cloud-init is enabled"
}

# Compact a qcow2 by rewriting it without zero blocks (no compression).
# fstrim inside the VM zeros free blocks; this step skips them in the output.
# Safe for Docker overlay2 — this is NOT compression, just sparse-block skipping.
compact_qcow2() {
  local img="$1"
  local tmp="${img}.compact.tmp"
  log "Compacting $img (skipping zero blocks, no compression)..."
  qemu-img convert -O qcow2 "$img" "$tmp"
  mv "$tmp" "$img"
  log "Compacted: $img ($(du -sh "$img" | cut -f1))"
}

# ── Stage 1: Base (Ubuntu + Chrome + VNC + ttyd + Docker + k3d + uds CLI) ────
if [ "${SKIP_BASE:-0}" != "1" ]; then
  log "Stage 1: base image (~20 min)..."
  packer init lab-base.qemu.pkr.hcl
  packer build -force lab-base.qemu.pkr.hcl
  BASE_IMAGE="output/base/lab-base.qcow2"
  fix_cloud_init "$BASE_IMAGE"
  compact_qcow2 "$BASE_IMAGE"
  log "Base image: $BASE_IMAGE ($(du -sh "$BASE_IMAGE" | cut -f1))"
else
  BASE_IMAGE="${BASE_IMAGE:-output/base/lab-base.qcow2}"
  [ -f "$BASE_IMAGE" ] || die "SKIP_BASE=1 but BASE_IMAGE not found: $BASE_IMAGE"
  warn "Skipping base build — using $BASE_IMAGE"
  fix_cloud_init "$BASE_IMAGE"
fi

# ── Stage 2: UDS Core playground ─────────────────────────────────────────────
if [ "${SKIP_UDS_CORE:-0}" != "1" ]; then
  log "Stage 2: UDS Core playground (~40 min — deploys full UDS Core)..."
  packer init lab-playground-uds-core.qemu.pkr.hcl
  packer build -force \
    -var "base_image=${BASE_IMAGE}" \
    lab-playground-uds-core.qemu.pkr.hcl
  UDS_CORE_IMAGE="output/uds-core/lab-playground-uds-core.qcow2"
  fix_cloud_init "$UDS_CORE_IMAGE"
  # No compact_qcow2 here — the uds-core image contains a stopped k3d cluster
  # whose containers must survive in Docker storage intact. Rewriting the qcow2
  # is safe in principle but unnecessary risk given prior overlay2 issues.
  log "UDS Core image: $UDS_CORE_IMAGE ($(du -sh "$UDS_CORE_IMAGE" | cut -f1))"
else
  warn "Skipping UDS Core build"
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "╔═══════════════════════════════════════════════════════════════╗"
echo "  qcow2 images built and patched (cloud-init enabled)."
echo "  Next: uds run build-vm-images-package && uds run deploy-bundle"
echo ""
echo "  Base:      ${BASE_IMAGE:-<skipped>}"
echo "  UDS Core:  ${UDS_CORE_IMAGE:-<skipped>}"
echo "╚═══════════════════════════════════════════════════════════════╝"
