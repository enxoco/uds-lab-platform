#!/bin/bash
# Builds the UDS Core playground image. Deploys k3d-core-slim-dev, waits
# for all pods to be ready, then stops the cluster cleanly before snapshot.
# On boot, setup.sh runs k3d cluster start for clean ordered startup.
set -euo pipefail

export HOME=/root

log() { echo "[$(date '+%H:%M:%S')] $*"; }

# ── Ensure Docker is running ───────────────────────────────────────────────────
log "Starting Docker..."
systemctl start docker
sleep 5

# ── Deploy UDS Core ────────────────────────────────────────────────────────────
log "Deploying k3d-core-slim-dev (this takes several minutes)..."
uds deploy k3d-core-slim-dev:latest --confirm 2>&1 | tee /var/log/uds-core-deploy.log

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

log "UDS Core playground build complete."
