#!/bin/bash
# Setup for the UDS Reference Package lab.
# The k3d cluster (uds) with UDS Core is already created from the playground snapshot.
# This script starts it, writes kubeconfig, synchronously clones the reference package
# repo (so it exists when the terminal opens), then patches the bundle and pre-builds
# the Zarf package in the background.
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

# ── Clone synchronously so /root/reference-package exists when terminal opens ──
# The bundle patch and Zarf pre-build happen in the background after this.
log "Cloning reference package..."
git clone --depth 1 https://github.com/uds-packages/reference-package /root/reference-package \
  >> /var/log/lab-setup/uds-setup.log 2>&1
log "Clone done."

# Mark lab ready — user can access terminal while background work continues
touch /var/log/lab-setup/ready
log "Cluster up — lab terminal ready."

# ── Background: wait for pods, patch bundle, pre-build zarf package ─────────────
{
  log "Waiting for UDS Core pods..."
  uds zarf tools kubectl wait --for=condition=Available deployment \
    --all --all-namespaces --timeout=300s >> /var/log/lab-setup/uds-setup.log 2>&1 || true
  uds zarf tools kubectl wait --for=condition=Ready statefulset \
    --all --all-namespaces --timeout=300s >> /var/log/lab-setup/uds-setup.log 2>&1 || true
  touch /var/log/lab-setup/pods-ready
  log "UDS Core pods ready."

  # Right-size the bundle for a single-node k3d cluster.
  # The upstream bundle configures Postgres for production (2 HA instances, 10Gi).
  # This cluster uses local-path provisioner and doesn't need HA.
  log "Patching bundle for single-node cluster..."
  cd /root/reference-package
  uds zarf tools yq e '
    (.packages[] | select(.name == "postgres-operator") | .overrides["postgres-operator"]["uds-postgres-config"].values[] | select(.path == "postgresql") | .value.numberOfInstances) = 1 |
    (.packages[] | select(.name == "postgres-operator") | .overrides["postgres-operator"]["uds-postgres-config"].values[] | select(.path == "postgresql") | .value.volume.size) = "5Gi" |
    (.packages[] | select(.name == "postgres-operator") | .overrides["postgres-operator"]["uds-postgres-config"].values[] | select(.path == "postgresql") | .value.volume.storageClass) = "local-path" |
    (.packages[] | select(.name == "postgres-operator") | .overrides["postgres-operator"]["uds-postgres-config"].values[] | select(.path == "postgresql") | .value.resources) = {"requests":{"cpu":"100m","memory":"256Mi"},"limits":{"cpu":"500m","memory":"512Mi"}}
  ' -i bundle/uds-bundle.yaml >> /var/log/lab-setup/uds-setup.log 2>&1
  log "Bundle patch applied."

  log "Pre-building Zarf package (warms image cache)..."
  uds zarf package create . --confirm --no-progress \
    >> /var/log/lab-setup/uds-setup.log 2>&1 && \
    touch /var/log/lab-setup/zarf-prebuild-done && \
    log "Zarf pre-build complete." || \
    log "Zarf pre-build failed — user will build fresh in step 4."
} &
