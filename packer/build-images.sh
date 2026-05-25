#!/bin/bash
# Build all lab images in order: base → playground-tools → playground-uds-core
# Each stage reads the previous stage's snapshot name from the packer manifest.
set -euo pipefail

cd "$(dirname "$0")"

log() { echo "[$(date '+%H:%M:%S')] $*"; }
die() { echo "ERROR: $*" >&2; exit 1; }

[ -n "${HCLOUD_TOKEN:-}" ] || die "HCLOUD_TOKEN is required"
command -v packer &>/dev/null || die "packer not found in PATH"
command -v jq &>/dev/null || die "jq not found in PATH (needed to parse manifests)"

snapshot_name_from_manifest() {
  local file="$1"
  jq -r '.builds[-1].artifact_id' "$file"
}

# ── Base image ─────────────────────────────────────────────────────────────────
if [ "${SKIP_BASE:-}" != "1" ]; then
  log "Building base image..."
  packer init lab-base.pkr.hcl
  packer build lab-base.pkr.hcl
  BASE_IMAGE=$(snapshot_name_from_manifest manifest.json)
  log "Base image: $BASE_IMAGE"
else
  [ -n "${BASE_IMAGE:-}" ] || die "SKIP_BASE=1 requires BASE_IMAGE to be set"
  log "Skipping base build, using BASE_IMAGE=$BASE_IMAGE"
fi

# ── Tools playground ───────────────────────────────────────────────────────────
if [ "${SKIP_TOOLS:-}" != "1" ]; then
  log "Building tools playground image..."
  BASE_IMAGE="$BASE_IMAGE" packer build lab-playground-tools.pkr.hcl
  TOOLS_IMAGE=$(snapshot_name_from_manifest manifest-playground-tools.json)
  log "Tools playground image: $TOOLS_IMAGE"
else
  [ -n "${TOOLS_IMAGE:-}" ] || die "SKIP_TOOLS=1 requires TOOLS_IMAGE to be set"
  log "Skipping tools build, using TOOLS_IMAGE=$TOOLS_IMAGE"
fi

# ── UDS Core playground ────────────────────────────────────────────────────────
if [ "${SKIP_UDS_CORE:-}" != "1" ]; then
  log "Building UDS Core playground image (this takes ~15 minutes)..."
  TOOLS_IMAGE="$TOOLS_IMAGE" packer build lab-playground-uds-core.pkr.hcl
  UDS_CORE_IMAGE=$(snapshot_name_from_manifest manifest-playground-uds-core.json)
  log "UDS Core playground image: $UDS_CORE_IMAGE"
fi

# ── Summary ────────────────────────────────────────────────────────────────────
echo ""
echo "╔══════════════════════════════════════════════════════════════╗"
echo "  Build complete. Snapshots auto-discovered at session creation."
echo ""
echo "  Base image (set as VM_IMAGE): ${BASE_IMAGE:-<not built>}"
[ -n "${TOOLS_IMAGE:-}" ]    && echo "  Tools playground:             $TOOLS_IMAGE"
[ -n "${UDS_CORE_IMAGE:-}" ] && echo "  UDS Core playground:          $UDS_CORE_IMAGE"
echo "╚══════════════════════════════════════════════════════════════╝"
