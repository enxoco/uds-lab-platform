#!/bin/bash
# Runs during VM bootstrap before the terminal is exposed.
# Installs tooling, pre-creates the sample Helm chart, and deploys UDS Core.
set -euo pipefail

LOG=/var/log/lab-setup/uds-setup.log
mkdir -p /var/log/lab-setup
touch "$LOG"

log() { echo "[$(date '+%H:%M:%S')] $*" | tee -a "$LOG"; }

# -- docker --------------------------
log "Installing docker..."
# Add Docker's official GPG key:
apt update
apt install ca-certificates curl
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc

# Add the repository to Apt sources:
sudo tee /etc/apt/sources.list.d/docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: $(. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}")
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/docker.asc
EOF

apt update
apt install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
log "docker installed: $(docker version | head -1)"

# ── k3d ────────────────────────────────────────────────────────────────────────
log "Installing k3d..."
curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash >> "$LOG" 2>&1
log "k3d installed: $(k3d version | head -1)"

# ── uds CLI ────────────────────────────────────────────────────────────────────
log "Installing uds CLI..."
UDS_TAG=$(curl -s https://api.github.com/repos/defenseunicorns/uds-cli/releases/latest \
  | grep '"tag_name"' | cut -d'"' -f4)
UDS_VER="v${UDS_TAG#v}"
curl -sSL \
  "https://github.com/defenseunicorns/uds-cli/releases/download/${UDS_TAG}/uds-cli_${UDS_VER}_Linux_amd64" \
  -o /usr/local/bin/uds
chmod +x /usr/local/bin/uds
echo 'alias kubectl="uds zarf tools kubectl"' >> /root/.bashrc
echo 'alias k="uds zarf tools kubectl"' >> /root/.bashrc
log "uds installed: $(uds version)"

# ── Sample app Helm chart ──────────────────────────────────────────────────────
log "Creating hello-uds Helm chart skeleton..."
mkdir -p /root/hello-uds/{chart/templates,values,manifests}

cat > /root/hello-uds/chart/Chart.yaml << 'EOF'
apiVersion: v2
name: hello-uds
description: Sample nginx app for UDS packaging quickstart
version: 0.1.0
appVersion: "1.27"
EOF

cat > /root/hello-uds/chart/values.yaml << 'EOF'
replicaCount: 1
image:
  repository: nginx
  tag: "1.27"
  pullPolicy: IfNotPresent
service:
  port: 80
config:
  domain: uds.dev
EOF

cat > /root/hello-uds/chart/templates/deployment.yaml << 'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hello-uds
  labels:
    app: hello-uds
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      app: hello-uds
  template:
    metadata:
      labels:
        app: hello-uds
    spec:
      containers:
        - name: hello-uds
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          ports:
            - containerPort: 80
          resources:
            requests:
              memory: "64Mi"
              cpu: "50m"
            limits:
              memory: "128Mi"
              cpu: "100m"
EOF

cat > /root/hello-uds/chart/templates/service.yaml << 'EOF'
apiVersion: v1
kind: Service
metadata:
  name: hello-uds
spec:
  selector:
    app: hello-uds
  ports:
    - port: 80
      targetPort: 80
EOF

log "Helm chart written to /root/hello-uds/chart/"

# ── UDS Core ───────────────────────────────────────────────────────────────────
log "Deploying UDS Core (k3d + Keycloak + Istio + Pepr) — takes 5-10 min..."
export HOME=/root
uds deploy k3d-core-slim-dev:latest --confirm >> "$LOG" 2>&1
log "UDS Core deployment complete."

# Signal ready for the lab server health check
touch /var/log/lab-setup/ready
