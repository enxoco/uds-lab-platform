#!/bin/bash
# Build and load the labserver image into k3d for local dev.
# Run from the repo root: ./docker/labserver/build.sh [cluster-name]
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TAG="ghcr.io/enxoco/uds-labserver:dev"
CLUSTER="${1:-uds-lab}"

docker build \
  --platform linux/amd64 \
  -t "$TAG" \
  -f "$REPO_ROOT/docker/labserver/Dockerfile" \
  "$REPO_ROOT"

k3d image import "$TAG" -c "$CLUSTER"

echo "Built and imported: $TAG"
echo ""
echo "Deploy:        kubectl apply -f deploy/dev/labserver.yaml"
echo "Port-forward:  kubectl port-forward pod/labserver -n uds-lab-platform 8080:8080"
