#!/bin/bash
# Minimal VM prep: creates the sample Helm chart skeleton.
# Users install docker/k3d/uds and deploy UDS Core themselves as lab steps.
set -euo pipefail

LOG=/var/log/lab-setup/uds-setup.log
mkdir -p /var/log/lab-setup
touch "$LOG"

log() { echo "[$(date '+%H:%M:%S')] $*" | tee -a "$LOG"; }

log "Preparing lab workspace..."

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

# Signal ready — setup is complete
touch /var/log/lab-setup/ready
log "Lab workspace ready."
