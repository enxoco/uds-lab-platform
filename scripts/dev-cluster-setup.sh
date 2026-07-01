#!/usr/bin/env bash
# scripts/dev-cluster-setup.sh
#
# Full wipe + rebuild of a local k3s cluster for UDS Lab Platform + KubeVirt dev.
# Requires: bare-metal host with /dev/kvm, uds, zarf, kubectl, curl, jq, docker
#
# Usage:
#   ./scripts/dev-cluster-setup.sh              # full wipe + rebuild
#   SKIP_WIPE=1 ./scripts/dev-cluster-setup.sh  # skip k3s reinstall (rebuild packages only)
#
# Golden PVCs require pre-built qcow2s from packer/build-images-qemu.sh.
# Run packer first, then:
#   SKIP_WIPE=1 \
#   SKIP_GOLDEN_PVC=0 \
#   BASE_QCOW2=packer/output/base/lab-base.qcow2 \
#   TOOLS_QCOW2=packer/output/tools/lab-playground-tools.qcow2 \
#   ./scripts/dev-cluster-setup.sh

set -euo pipefail

# ── Config (override via env) ────────────────────────────────────────────────

KUBEVIRT_PKG_DIR="${KUBEVIRT_PKG_DIR:-$HOME/src/github.com/uds-packages/kubevirt}"
LAB_PLATFORM_DIR="${LAB_PLATFORM_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"
VM_NAMESPACES="${VM_NAMESPACES:-uds-lab-vms}"
METALLB_VERSION="${METALLB_VERSION:-v0.14.9}"
SKIP_WIPE="${SKIP_WIPE:-0}"

# Golden PVC population — runs AFTER bundle deploy.
# Packer builds take 40+ min; skip by default and run after packer is done.
# qcow2s must be patched (no /etc/cloud/cloud-init.disabled) — packer/build-images-qemu.sh does this automatically.
SKIP_GOLDEN_PVC="${SKIP_GOLDEN_PVC:-1}"
BASE_QCOW2="${BASE_QCOW2:-}"
TOOLS_QCOW2="${TOOLS_QCOW2:-}"
UDS_CORE_QCOW2="${UDS_CORE_QCOW2:-}"

# ── Helpers ──────────────────────────────────────────────────────────────────

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BOLD='\033[1m'; NC='\033[0m'
log()  { echo -e "${GREEN}▶${NC} $*"; }
warn() { echo -e "${YELLOW}⚠ $*${NC}"; }
die()  { echo -e "${RED}✗ $*${NC}" >&2; exit 1; }
step() { echo -e "\n${BOLD}══ $* ══${NC}"; }

# ── Preflight ────────────────────────────────────────────────────────────────

preflight() {
  step "Preflight"
  for cmd in kubectl zarf uds curl jq ip docker; do
    command -v "$cmd" &>/dev/null || die "Missing required tool: $cmd"
  done
  ls /dev/kvm &>/dev/null || die "/dev/kvm not found — enable KVM in BIOS (AMD-V / Intel VT-x)"
  [ -d "$KUBEVIRT_PKG_DIR" ] || die "KubeVirt package dir not found: $KUBEVIRT_PKG_DIR"
  log "All preflight checks passed"
}

# ── Step 1: Wipe k3s ─────────────────────────────────────────────────────────

wipe_k3s() {
  step "Wipe k3s"
  if [ -f /usr/local/bin/k3s-uninstall.sh ]; then
    log "Uninstalling existing k3s..."
    sudo /usr/local/bin/k3s-uninstall.sh
    sleep 3
  else
    warn "k3s-uninstall.sh not found — assuming clean state"
  fi
}

# ── Step 2: Install k3s ──────────────────────────────────────────────────────

install_k3s() {
  step "Install k3s"
  # --disable=traefik  → avoids gateway-api CRD ownership conflict with UDS Core
  # --disable=servicelb → replaced by MetalLB so admin+tenant gateways get separate IPs
  log "Installing k3s (traefik+servicelb disabled)..."
  curl -sfL https://get.k3s.io | sh -s - --disable=traefik --disable=servicelb

  mkdir -p "$HOME/.kube"
  sudo cp /etc/rancher/k3s/k3s.yaml "$HOME/.kube/config"
  sudo chown "$(id -u):$(id -g)" "$HOME/.kube/config"
  export KUBECONFIG="$HOME/.kube/config"

  log "Waiting for API server..."
  until kubectl get nodes &>/dev/null; do sleep 2; done

  log "Waiting for node Ready..."
  kubectl wait node --all --for=condition=Ready --timeout=120s
  kubectl get nodes
}

# ── Step 3: MetalLB (BEFORE zarf init — no Zarf agent yet, raw apply is safe) ──

install_metallb() {
  step "MetalLB $METALLB_VERSION"

  log "Applying MetalLB manifests..."
  kubectl apply -f "https://raw.githubusercontent.com/metallb/metallb/${METALLB_VERSION}/config/manifests/metallb-native.yaml"
  kubectl wait -n metallb-system deployment/controller --for=condition=Available --timeout=120s
  kubectl rollout status daemonset/speaker -n metallb-system --timeout=60s || true

  # Auto-detect the host's outbound interface IP and use the .200-.250 range
  LOCAL_IP=$(ip route get 8.8.8.8 2>/dev/null | grep -oP 'src \K[^ ]+' || true)
  [ -n "$LOCAL_IP" ] || die "Cannot detect local IP. Set METALLB_RANGE=x.x.x.a-x.x.x.b and rerun."
  SUBNET=$(echo "$LOCAL_IP" | cut -d. -f1-3)
  METALLB_RANGE="${METALLB_RANGE:-${SUBNET}.200-${SUBNET}.250}"
  log "MetalLB IP pool: $METALLB_RANGE"

  kubectl apply -f - <<EOF
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: local-pool
  namespace: metallb-system
spec:
  addresses:
  - ${METALLB_RANGE}
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: local-adv
  namespace: metallb-system
EOF
  log "MetalLB configured"
}

# ── Step 4: zarf init ────────────────────────────────────────────────────────

run_zarf_init() {
  step "zarf init"
  zarf init --confirm
}

# ── Step 5: Build KubeVirt package ───────────────────────────────────────────

build_kubevirt() {
  step "Build KubeVirt package"
  cd "$KUBEVIRT_PKG_DIR"

  # Skip rebuild when SKIP_WIPE=1 and a package tarball already exists.
  # On a fresh cluster wipe we always rebuild to pick up any upstream changes.
  if [ "${SKIP_WIPE}" = "1" ]; then
    EXISTING_PKG=$(ls zarf-package-kubevirt-amd64-*-upstream.tar.zst 2>/dev/null | tail -1)
    if [ -n "$EXISTING_PKG" ]; then
      log "SKIP_WIPE=1 and package exists ($EXISTING_PKG) — skipping KubeVirt rebuild"
      return
    fi
  fi

  log "Building KubeVirt package (upstream flavor)..."
  zarf package create . -f upstream --confirm --skip-sbom

  KUBEVIRT_PKG=$(ls zarf-package-kubevirt-amd64-*-upstream.tar.zst 2>/dev/null | tail -1)
  [ -n "$KUBEVIRT_PKG" ] || die "KubeVirt package tarball not found after build"
  log "Built: $KUBEVIRT_PKG"
}

# ── Step 6: Create + deploy bundle ───────────────────────────────────────────

deploy_bundle() {
  step "Create + deploy bundle"
  cd "$LAB_PLATFORM_DIR"

  log "Building CDI Zarf package..."
  (cd packages/cdi && zarf package create . --confirm --skip-sbom)

  log "Building lab-server Docker image..."
  docker build -t ghcr.io/enxoco/uds-lab-platform:dev-local .
  docker save ghcr.io/enxoco/uds-lab-platform:dev-local -o lab-server.tar

  log "Building uds-lab-platform Zarf package..."
  zarf package create . --confirm --skip-sbom

  log "Creating bundle..."
  uds create bundle/ --confirm

  BUNDLE=$(ls bundle/uds-bundle-uds-lab-platform-*.tar.zst 2>/dev/null | tail -1)
  [ -n "$BUNDLE" ] || die "Bundle tarball not found after create"

  log "Deploying bundle: $BUNDLE"
  uds deploy "$BUNDLE" --confirm

  log "Waiting for KubeVirt to be Available..."
  kubectl -n kubevirt wait kubevirt kubevirt --for=condition=Available --timeout=300s

  log "Waiting for CDI to be Deployed..."
  local deadline=$(( $(date +%s) + 180 ))
  while true; do
    local phase
    phase=$(kubectl get cdi cdi -o jsonpath='{.status.phase}' 2>/dev/null || echo "")
    [ "$phase" = "Deployed" ] && { log "CDI Deployed"; break; }
    [ "$(date +%s)" -gt "$deadline" ] && die "CDI not Deployed after 3m — check: kubectl get pods -n cdi"
    echo -n "."
    sleep 3
  done
  echo ""
}

# CDI is included in the bundle as packages/cdi — no separate deploy step needed.

# ── Step 7: Golden PVCs (optional) ───────────────────────────────────────────

populate_golden_pvcs() {
  step "Golden PVCs"
  if [ "${SKIP_GOLDEN_PVC}" = "1" ]; then
    warn "SKIP_GOLDEN_PVC=1 — skipping golden PVC population"
    warn "Build qcow2s first (packer/build-images-qemu.sh), then rerun:"
    warn "  SKIP_WIPE=1 SKIP_GOLDEN_PVC=0 \\"
    warn "  BASE_QCOW2=packer/output/base/lab-base.qcow2 \\"
    warn "  TOOLS_QCOW2=packer/output/tools/lab-playground-tools.qcow2 \\"
    warn "  ./scripts/dev-cluster-setup.sh"
    return
  fi

  cd "$LAB_PLATFORM_DIR"

  # Only pass qcow2 vars for files that actually exist — skip missing tiers silently.
  local import_args=()
  [ -n "${BASE_QCOW2:-}"     ] && [ -f "$BASE_QCOW2"     ] && import_args+=("BASE_QCOW2=$BASE_QCOW2")
  [ -n "${TOOLS_QCOW2:-}"    ] && [ -f "$TOOLS_QCOW2"    ] && import_args+=("TOOLS_QCOW2=$TOOLS_QCOW2")
  [ -n "${UDS_CORE_QCOW2:-}" ] && [ -f "$UDS_CORE_QCOW2" ] && import_args+=("UDS_CORE_QCOW2=$UDS_CORE_QCOW2")

  if [ ${#import_args[@]} -eq 0 ]; then
    warn "No qcow2 files found at expected paths — skipping golden PVC import"
    warn "Run 'uds run build-images' to build them, then re-run with SKIP_WIPE=1"
    return
  fi

  env "${import_args[@]}" GOLDEN_NAMESPACE="$VM_NAMESPACES" \
    ./scripts/create-golden-pvc.sh
}

# ── Step 8: Verify ───────────────────────────────────────────────────────────

verify() {
  step "Verify"
  kubectl get nodes
  kubectl -n kubevirt get kubevirt kubevirt \
    -o jsonpath='KubeVirt status: {.status.phase}{"\n"}'
  kubectl get pods -A | grep -v Running | grep -v Completed | grep -v NAME || true
}

# ── Main ─────────────────────────────────────────────────────────────────────

main() {
  preflight

  if [ "${SKIP_WIPE}" = "0" ]; then
    wipe_k3s
    install_k3s
    install_metallb    # BEFORE zarf init — no Zarf agent yet, raw kubectl is safe
    run_zarf_init
  else
    warn "SKIP_WIPE=1 — skipping k3s reinstall, metallb, and zarf init"
    export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/config}"
  fi

  build_kubevirt
  deploy_bundle
  populate_golden_pvcs
  verify

  echo ""
  if [ "${SKIP_GOLDEN_PVC}" = "1" ]; then
    log "Cluster ready (no golden PVCs). Build qcow2s then:"
    log "  cd packer && ./build-images-qemu.sh"
    log "  cd .. && SKIP_WIPE=1 SKIP_GOLDEN_PVC=0 \\"
    log "    BASE_QCOW2=packer/output/base/lab-base.qcow2 \\"
    log "    TOOLS_QCOW2=packer/output/tools/lab-playground-tools.qcow2 \\"
    log "    ./scripts/dev-cluster-setup.sh"
  else
    log "Cluster ready with golden PVCs. Create a LabSession CR to test:"
    log "  kubectl apply -f - <<'EOF'"
    log "  apiVersion: lab.uds.dev/v1alpha1"
    log "  kind: LabSession"
    log "  metadata:"
    log "    name: test-s01"
    log "    namespace: default"
    log "  spec:"
    log "    scenarioRef: playground-tools"
    log "    userID: testuser"
    log "  EOF"
  fi
}

main "$@"
