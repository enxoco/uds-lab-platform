#!/bin/bash
# k3d containers persist in the snapshot. Restart Docker, wait for cluster,
# then regenerate kubeconfig (stale after snapshot restore).
set -euo pipefail
export HOME=/root
mkdir -p /var/log/lab-setup /root/.kube

log() { echo "[$(date '+%H:%M:%S')] $*" | tee -a /var/log/lab-setup/uds-setup.log; }

log "Starting Docker..."
systemctl start docker

log "Waiting for k3d cluster..."
for i in $(seq 1 60); do
  if k3d kubeconfig get uds > /root/.kube/config 2>/dev/null; then
    export KUBECONFIG=/root/.kube/config
    if uds zarf tools kubectl get nodes --request-timeout=5s &>/dev/null; then
      log "Cluster ready."
      break
    fi
  fi
  sleep 5
done

log "Regenerating kubeconfig..."
k3d kubeconfig get uds > /root/.kube/config
chmod 600 /root/.kube/config

touch /var/log/lab-setup/ready
