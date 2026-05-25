#!/bin/bash
# k3d containers persist in the snapshot. Restart Docker and wait for
# cluster to come back up before marking ready.
set -euo pipefail
export HOME=/root
mkdir -p /var/log/lab-setup

log() { echo "[$(date '+%H:%M:%S')] $*" | tee -a /var/log/lab-setup/uds-setup.log; }

log "Starting Docker..."
systemctl start docker

log "Waiting for k3d cluster..."
for i in $(seq 1 60); do
  if uds zarf tools kubectl get nodes --request-timeout=5s &>/dev/null; then
    log "Cluster ready."
    break
  fi
  sleep 5
done

touch /var/log/lab-setup/ready
