#!/bin/bash
# Pre-clone the reference package and warm the Zarf image cache in the background.
# The cluster is already running (playground-uds-core snapshot). Lab terminal is
# marked ready immediately so users can start reading while pre-build runs.
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

# Show an initialization notice until pods are fully ready
cat > /etc/profile.d/uds-init-status.sh << 'EOF'
if [ ! -f /var/log/lab-setup/pods-ready ]; then
  echo ""
  echo "  UDS Core pods are still initializing (~2-3 min)."
  echo "  Monitor: uds zarf tools kubectl get pods -A"
  echo ""
fi
EOF

# Mark lab ready — user can access terminal while background work continues
touch /var/log/lab-setup/ready
log "Cluster up — lab terminal ready."

# ── Background: wait for pods, pre-clone repo, pre-build zarf package ─────────
{
  log "Waiting for UDS Core pods..."
  uds zarf tools kubectl wait --for=condition=Available deployment \
    --all --all-namespaces --timeout=300s >> /var/log/lab-setup/uds-setup.log 2>&1 || true
  uds zarf tools kubectl wait --for=condition=Ready statefulset \
    --all --all-namespaces --timeout=300s >> /var/log/lab-setup/uds-setup.log 2>&1 || true
  touch /var/log/lab-setup/pods-ready
  log "UDS Core pods ready."

  log "Cloning reference package..."
  git clone --depth 1 https://github.com/uds-packages/reference-package /opt/reference-package-cache \
    >> /var/log/lab-setup/uds-setup.log 2>&1
  log "Clone done."

  log "Pre-building Zarf package (warms image cache)..."
  cd /opt/reference-package-cache
  uds zarf package create . --confirm --no-progress \
    >> /var/log/lab-setup/uds-setup.log 2>&1 && \
    touch /var/log/lab-setup/zarf-prebuild-done && \
    log "Zarf pre-build complete." || \
    log "Zarf pre-build failed — user will build fresh in step 4."
} &
