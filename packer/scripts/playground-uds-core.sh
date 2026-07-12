#!/bin/bash
# Builds the UDS Core playground image. Deploys k3d-core-slim-dev, waits
# for all pods to be ready, then stops the cluster cleanly before snapshot.
# On boot, setup.sh runs k3d cluster start for clean ordered startup.
set -euo pipefail

export HOME=/root

log() { echo "[$(date '+%H:%M:%S')] $*"; }

# Pinned bundle version — bump intentionally to upgrade UDS Core.
# Using :latest caused a silent 1.7→1.8 breakage; always pin.
UDS_CORE_BUNDLE="oci://ghcr.io/defenseunicorns/packages/uds/bundles/k3d-core-slim-dev:1.7.0"

# ── Ensure Docker is running ───────────────────────────────────────────────────
log "Starting Docker..."
systemctl start docker
sleep 5

# ── Deploy UDS Core (single-node optimized) ────────────────────────────────────
# These overrides right-size the deployment for a single-node dev VM.
# They mirror the values in bundle/uds-bundle.yaml.
log "Deploying ${UDS_CORE_BUNDLE} (single-node optimized)..."
uds deploy "${UDS_CORE_BUNDLE}" --confirm \
  --set ISTIOD_CPU_REQUEST=100m \
  --set ISTIOD_MEMORY_REQUEST=512Mi \
  --set PROXY_CPU_REQUEST=10m \
  --set PROXY_CPU_LIMIT=2000m \
  --set PROXY_MEMORY_REQUEST=40Mi \
  --set PROXY_MEMORY_LIMIT=1024Mi \
  --set PEPR_WATCHER_CPU_REQUEST=50m \
  --set PEPR_WATCHER_MEMORY_REQUEST=128Mi \
  --set PEPR_ADMISSION_CPU_REQUEST=50m \
  --set PEPR_ADMISSION_MEMORY_REQUEST=128Mi \
  --set AUTHSERVICE_REPLICA_COUNT=1 \
  --set KEYCLOAK_HA=false \
  --set KEYCLOAK_CPU_REQUEST=100m \
  --set KEYCLOAK_CPU_LIMIT=1000m \
  --set KEYCLOAK_MEMORY_REQUEST=512Mi \
  --set KEYCLOAK_MEMORY_LIMIT=1Gi \
  --set KEYCLOAK_WAYPOINT_HPA_ENABLED=false \
  --set KEYCLOAK_WAYPOINT_CPU_REQUEST=100m \
  --set KEYCLOAK_WAYPOINT_MEMORY_REQUEST=64Mi \
  2>&1 | tee /var/log/uds-core-deploy.log

# ── Wait for all pods to be ready ─────────────────────────────────────────────
log "Waiting for all pods to be ready..."
uds zarf tools kubectl wait --for=condition=Ready pods --all --all-namespaces --timeout=300s

log "Cluster state:"
uds zarf tools kubectl get pods -A

# ── Stop cluster cleanly before snapshot ──────────────────────────────────────
# Stopped cluster starts cleanly on boot via k3d cluster start — avoids
# crash loops from Docker restarting containers in arbitrary order.
log "Stopping cluster cleanly for snapshot..."
k3d cluster stop uds
# WARNING: do NOT prune containers or images here. The stopped k3d containers
# must exist in the snapshot — k3d cluster start on VM boot restarts them.
# Pruning removes them and leaves k3d with nothing to start.

log "UDS Core playground build complete."
