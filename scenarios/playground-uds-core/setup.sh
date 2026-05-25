#!/bin/bash
# Start k3d cluster (stopped cleanly in snapshot), mark ready once cluster
# is up, then wait for pods in background.
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

# ── Mark lab ready so user can access terminal ────────────────────────────────
log "Cluster up — marking lab ready."

# Show an initialization notice until pods are fully ready
cat > /etc/profile.d/uds-init-status.sh << 'EOF'
if [ ! -f /var/log/lab-setup/pods-ready ]; then
  echo ""
  echo "  ⚡ UDS Core pods are still initializing (~2-3 min)."
  echo "     Monitor progress: uds zarf tools monitor"
  echo ""
fi
EOF

touch /var/log/lab-setup/ready

# ── Wait for pods in background, remove notice when done ─────────────────────
{
  uds zarf tools kubectl wait --for=condition=Available deployment \
    --all --all-namespaces --timeout=300s >> /var/log/lab-setup/uds-setup.log 2>&1 || true
  uds zarf tools kubectl wait --for=condition=Ready statefulset \
    --all --all-namespaces --timeout=300s >> /var/log/lab-setup/uds-setup.log 2>&1 || true
  touch /var/log/lab-setup/pods-ready
  log "All pods ready."
} &
