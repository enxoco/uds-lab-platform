#!/bin/bash
# Build the lab container image for the pod provider (dev / macOS backend).
# Run from anywhere: ./docker/lab/build.sh [tag] [cluster-name]
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TAG="${1:-ghcr.io/enxoco/uds-lab:dev}"
CLUSTER="${2:-uds-lab}"

docker build \
  --platform linux/amd64 \
  -t "$TAG" \
  -f "$REPO_ROOT/docker/lab/Dockerfile" \
  "$REPO_ROOT"

k3d image import "$TAG" -c "$CLUSTER"

echo "Built and imported: $TAG"
echo ""
echo "Smoke-test (standalone):"
echo "  docker run --rm -p 7681:7681 -p 7680:7680 $TAG"
