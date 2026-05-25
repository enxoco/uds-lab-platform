#!/bin/bash
# Builds the UDS Core playground image. Deploys k3d-core-slim-dev on top
# of the tools playground image, then snapshots the running cluster state.
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

log "Verifying cluster..."
uds zarf tools kubectl get nodes
uds zarf tools kubectl get pods -A | head -30

# ── Persist kubeconfig ─────────────────────────────────────────────────────────
mkdir -p /root/.kube
uds zarf tools kubectl config view --raw > /root/.kube/config
chmod 600 /root/.kube/config

# ── Mark as pre-provisioned ────────────────────────────────────────────────────
mkdir -p /var/log/lab-setup
touch /var/log/lab-setup/ready

log "UDS Core playground build complete."
