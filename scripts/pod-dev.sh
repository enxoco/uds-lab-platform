#!/usr/bin/env bash
# Full local dev reset for the pod provider (macOS / no nested-virt).
#
# Usage:
#   ./scripts/pod-dev.sh              # full wipe + rebuild
#   ./scripts/pod-dev.sh --skip-wipe  # keep cluster, rebuild images + redeploy
#   ./scripts/pod-dev.sh --skip-images # full wipe, skip image rebuild
#
# After this script exits, run the operator in a second terminal:
#   PROVIDER_TYPE=pod LAB_IMAGE=ghcr.io/enxoco/uds-lab:dev \
#   VM_NAMESPACE=uds-lab-vms SERVER_NAMESPACE=uds-lab-platform \
#     go run ./cmd/laboperator/
#
# Then open http://localhost:8080 in your browser.

set -euo pipefail
cd "$(dirname "$0")/.."

CLUSTER="${CLUSTER:-uds-lab}"
LAB_TAG="ghcr.io/enxoco/uds-lab:dev"
SERVER_TAG="ghcr.io/enxoco/uds-labserver:dev"
SKIP_WIPE=0
SKIP_IMAGES=0

for arg in "$@"; do
  case "$arg" in
    --skip-wipe)   SKIP_WIPE=1 ;;
    --skip-images) SKIP_IMAGES=1 ;;
    *) echo "Unknown arg: $arg" >&2; exit 1 ;;
  esac
done

# ── 1. Cluster ────────────────────────────────────────────────────────────────
if [ "$SKIP_WIPE" = "0" ]; then
  echo "==> Deleting cluster '$CLUSTER' (if it exists)..."
  k3d cluster delete "$CLUSTER" 2>/dev/null || true

  echo "==> Creating cluster '$CLUSTER'..."
  k3d cluster create "$CLUSTER" \
    --agents 0 \
    --no-lb \
    --timeout 120s
fi

echo "==> Ensuring namespaces..."
kubectl create namespace uds-lab-vms      --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace uds-lab-platform --dry-run=client -o yaml | kubectl apply -f -

echo "==> Installing LabSession CRD..."
kubectl apply -f deploy/crd/

# ── 2. Images ─────────────────────────────────────────────────────────────────
if [ "$SKIP_IMAGES" = "0" ]; then
  echo "==> Building lab image ($LAB_TAG)..."
  docker build \
    --platform linux/amd64 \
    -t "$LAB_TAG" \
    -f docker/lab/Dockerfile \
    .

  echo "==> Building labserver image ($SERVER_TAG)..."
  docker build \
    --platform linux/amd64 \
    -t "$SERVER_TAG" \
    -f docker/labserver/Dockerfile \
    .

  echo "==> Importing images into k3d..."
  k3d image import "$LAB_TAG" "$SERVER_TAG" -c "$CLUSTER"
else
  echo "==> Skipping image builds (--skip-images)"
fi

# ── 3. Deploy labserver ───────────────────────────────────────────────────────
echo "==> Deploying labserver..."
kubectl delete pod labserver -n uds-lab-platform --ignore-not-found
kubectl apply -f deploy/dev/labserver.yaml

echo "==> Waiting for labserver pod to be Running..."
kubectl wait pod/labserver \
  -n uds-lab-platform \
  --for=condition=Ready \
  --timeout=120s

# ── 4. Port-forward ───────────────────────────────────────────────────────────
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Stack ready.  Open a second terminal and run the operator:"
echo ""
echo "  PROVIDER_TYPE=pod \\"
echo "  LAB_IMAGE=$LAB_TAG \\"
echo "  VM_NAMESPACE=uds-lab-vms \\"
echo "  SERVER_NAMESPACE=uds-lab-platform \\"
echo "    go run ./cmd/laboperator/"
echo ""
echo "  Then start the port-forward (keep this terminal open):"
echo ""
echo "  kubectl port-forward --address 0.0.0.0 pod/labserver -n uds-lab-platform 8080:8080"
echo ""
echo "  Browser: http://localhost:8080"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Auto-start port-forward in the foreground so this terminal stays useful.
echo "==> Starting port-forward (Ctrl+C to stop)..."
kubectl port-forward --address 0.0.0.0 pod/labserver -n uds-lab-platform 8080:8080
