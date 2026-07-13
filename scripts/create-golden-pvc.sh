#!/usr/bin/env bash
# Populate golden PVCs from local qcow2 files via CDI HTTP import.
#
# For each image tier, this script:
#   1. Serves the qcow2 via a temporary python3 HTTP server (CDI needs HTTP URL)
#   2. Creates a CDI DataVolume with source.http pointing at it
#   3. Waits for the DataVolume to reach Succeeded
#   4. Stops the HTTP server
#
# Usage:
#   BASE_QCOW2=packer/output/base/lab-base.qcow2 \
#   UDS_CORE_QCOW2=packer/output/uds-core/lab-playground-uds-core.qcow2 \
#   ./scripts/create-golden-pvc.sh
#
# Skip a tier by leaving its env var unset.
# Override namespace: GOLDEN_NAMESPACE=uds-lab-vms (default)

set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
log()  { echo -e "${GREEN}[$(date '+%H:%M:%S')]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
die()  { echo -e "${RED}[ERROR]${NC} $*" >&2; exit 1; }

GOLDEN_NAMESPACE="${GOLDEN_NAMESPACE:-uds-lab-vms}"
# Each tier gets its own port — no kill/rebind race between imports.
PORT_BASE="${PORT_BASE:-18888}"
PORT_UDS_CORE="${PORT_UDS_CORE:-18890}"
HTTP_PIDS=()

cleanup() {
  for pid in "${HTTP_PIDS[@]+"${HTTP_PIDS[@]}"}"; do
    if kill -0 "$pid" 2>/dev/null; then
      # Kill the whole process group so the python3 child doesn't orphan and hold the port.
      kill -- "-$pid" 2>/dev/null || kill "$pid" 2>/dev/null || true
    fi
  done
}
trap cleanup EXIT

# ── Preflight ─────────────────────────────────────────────────────────────────
command -v kubectl &>/dev/null || die "kubectl not found"
command -v python3  &>/dev/null || die "python3 not found (needed for HTTP server)"

# Kill any stale HTTP servers from a previous run on our ports.
# fuser requires no elevated privileges unlike ss -tlnp pid info.
for port in "$PORT_BASE" "$PORT_UDS_CORE"; do
  if command -v fuser &>/dev/null; then
    fuser -k "${port}/tcp" &>/dev/null && warn "Killed stale server on port $port" || true
  elif command -v lsof &>/dev/null; then
    pid=$(lsof -ti "tcp:${port}" 2>/dev/null || true)
    if [ -n "$pid" ]; then
      warn "Port $port in use by PID $pid — killing stale server"
      kill -- "-$pid" 2>/dev/null || kill "$pid" 2>/dev/null || true
    fi
  fi
  sleep 0.5
done

kubectl get namespace "$GOLDEN_NAMESPACE" &>/dev/null \
  || die "namespace $GOLDEN_NAMESPACE not found — run dev-cluster-setup.sh first"

# Verify CDI is installed
kubectl get crd datavolumes.cdi.kubevirt.io &>/dev/null \
  || die "CDI DataVolume CRD not found — deploy CDI package first"

# Wait for CDI to be fully ready.
# CDI uses an operator model: cdi-operator reconciles the CDI CR which then
# creates the cdi-controller/cdi-apiserver/cdi-uploadproxy deployments.
# Wait on the CDI CR condition rather than a specific deployment.
log "Waiting for CDI to be Available (CR condition)..."
cdi_deadline=$(( $(date +%s) + 180 ))
while true; do
  cdi_phase=$(kubectl get cdi cdi -o jsonpath='{.status.phase}' 2>/dev/null || echo "")
  if [ "$cdi_phase" = "Deployed" ]; then
    log "CDI phase: Deployed"
    break
  fi
  if [ "$(date +%s)" -gt "$cdi_deadline" ]; then
    die "CDI not Deployed after 3m (phase=$cdi_phase) — check: kubectl get pods -n cdi"
  fi
  echo -n "."
  sleep 3
done
echo ""

# Detect host IP reachable from cluster pods.
# Allow override via HTTP_HOST_IP env var.
if [ -n "${HTTP_HOST_IP:-}" ]; then
  HOST_IP="$HTTP_HOST_IP"
else
  HOST_IP=$(ip route get 1.1.1.1 2>/dev/null \
    | awk '/src/{for(i=1;i<=NF;i++) if($i=="src"){print $(i+1); exit}}')
fi
[ -n "$HOST_IP" ] || die "could not detect host IP — set HTTP_HOST_IP=<ip> and retry"
log "Host IP for CDI HTTP import: $HOST_IP"

# ── Import one image ──────────────────────────────────────────────────────────
import_image() {
  local tier="$1"     # base | uds-core
  local pvc_name="$2" # golden-base | golden-uds-core
  local qcow2="$3"    # path to qcow2 file
  local port="$4"     # dedicated HTTP port for this tier

  [ -f "$qcow2" ] || die "qcow2 not found: $qcow2"

  local qcow2_dir
  qcow2_dir="$(cd "$(dirname "$qcow2")" && pwd)"
  local qcow2_file
  qcow2_file="$(basename "$qcow2")"

  log "Importing $tier → $pvc_name from $qcow2 (port $port)..."

  # setsid puts python3 in its own process group so cleanup can kill the whole group.
  # Each tier uses a dedicated port — no kill/rebind race.
  setsid python3 -m http.server "$port" --directory "$qcow2_dir" &>/dev/null &
  local srv_pid=$!
  HTTP_PIDS+=("$srv_pid")
  sleep 2  # give server a moment to bind

  # Verify server is actually up before handing URL to CDI
  curl -sf --max-time 10 "http://127.0.0.1:${port}/${qcow2_file}" -o /dev/null \
    || die "HTTP server on port $port not serving $qcow2_file — check if port is in use"

  local image_url="http://${HOST_IP}:${port}/${qcow2_file}"

  # Size from qcow2 (virtual size, rounded up to nearest Gi)
  local virtual_size_bytes
  virtual_size_bytes=$(python3 -c "
import subprocess, json, sys
r = subprocess.run(['qemu-img', 'info', '--output=json', '${qcow2}'], capture_output=True)
info = json.loads(r.stdout)
print(info['virtual-size'])
" 2>/dev/null || echo "0")

  local disk_size="80Gi"
  if [ "$virtual_size_bytes" -gt 0 ] 2>/dev/null; then
    local gi=$(( (virtual_size_bytes + (1024**3 - 1)) / (1024**3) ))
    disk_size="${gi}Gi"
  fi
  log "Disk size for $pvc_name: $disk_size"

  # Delete existing golden PVC if present (re-import)
  kubectl delete datavolume "$pvc_name" -n "$GOLDEN_NAMESPACE" --ignore-not-found=true

  # Create DataVolume
  kubectl apply -f - <<EOF
apiVersion: cdi.kubevirt.io/v1beta1
kind: DataVolume
metadata:
  name: ${pvc_name}
  namespace: ${GOLDEN_NAMESPACE}
  labels:
    lab.uds.dev/golden-pvc: "true"
    lab.uds.dev/tier: ${tier}
spec:
  source:
    http:
      url: "${image_url}"
  pvc:
    accessModes:
      - ReadWriteOnce
    resources:
      requests:
        storage: ${disk_size}
EOF

  log "Waiting for DataVolume $pvc_name to reach Succeeded (may take several minutes)..."
  local deadline=$(( $(date +%s) + 1800 ))  # 30 min timeout
  while true; do
    local phase
    phase=$(kubectl get datavolume "$pvc_name" -n "$GOLDEN_NAMESPACE" \
      -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")

    case "$phase" in
      Succeeded)
        log "$pvc_name import complete"
        break
        ;;
      Failed)
        die "$pvc_name import failed. Check CDI controller logs: kubectl logs -n cdi -l cdi.kubevirt.io=cdi-controller"
        ;;
      *)
        # Pending / ImportScheduled / ImportInProgress / empty (not yet reconciled) — keep waiting
        ;;
    esac

    if [ "$(date +%s)" -gt "$deadline" ]; then
      die "$pvc_name import timed out after 30m (phase=$phase)"
    fi
    echo -n "."
    sleep 10
  done
  echo ""
}

# ── Run imports ───────────────────────────────────────────────────────────────
imported=0

if [ -n "${BASE_QCOW2:-}" ]; then
  import_image "base" "golden-base" "$BASE_QCOW2" "$PORT_BASE"
  imported=$(( imported + 1 ))
else
  warn "BASE_QCOW2 not set — skipping base tier"
fi

if [ -n "${UDS_CORE_QCOW2:-}" ]; then
  import_image "uds-core" "golden-uds-core" "$UDS_CORE_QCOW2" "$PORT_UDS_CORE"
  imported=$(( imported + 1 ))
else
  warn "UDS_CORE_QCOW2 not set — skipping uds-core tier"
fi

[ "$imported" -gt 0 ] || die "No qcow2 files provided. Set BASE_QCOW2 or UDS_CORE_QCOW2."

echo ""
echo "╔═══════════════════════════════════════════════════════════════╗"
echo "  Golden PVCs ready in namespace: ${GOLDEN_NAMESPACE}"
echo ""
echo "  Update chart/values.yaml goldenPVCs section:"
[ -n "${BASE_QCOW2:-}" ]     && echo "    base:     golden-base"
[ -n "${UDS_CORE_QCOW2:-}" ] && echo "    uds-core: golden-uds-core"
echo "╚═══════════════════════════════════════════════════════════════╝"
