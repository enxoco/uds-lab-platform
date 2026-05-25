#!/bin/bash
# Start the k3d cluster (stopped cleanly in snapshot) and wait for
# all pods to be ready before marking the lab environment ready.
set -euo pipefail
export HOME=/root
mkdir -p /var/log/lab-setup /root/.kube

log() { echo "[$(date '+%H:%M:%S')] $*" | tee -a /var/log/lab-setup/uds-setup.log; }

log "Starting Docker..."
systemctl start docker
sleep 3

log "Starting k3d cluster..."
k3d cluster start uds

log "Writing kubeconfig..."
k3d kubeconfig get uds > /root/.kube/config
chmod 600 /root/.kube/config
export KUBECONFIG=/root/.kube/config

log "Waiting for pods to be ready..."
uds zarf tools kubectl wait --for=condition=Ready pods --all --all-namespaces --timeout=300s 2>&1 | \
  tee -a /var/log/lab-setup/uds-setup.log

log "Lab ready."
touch /var/log/lab-setup/ready
