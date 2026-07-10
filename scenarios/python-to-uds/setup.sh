#!/bin/bash
# Setup for the Python-to-UDS scenario.
# The k3d cluster (uds) with UDS Core is already created from the playground snapshot.
# This script starts the cluster, scaffolds /root/myapp synchronously (so files are
# present when the terminal opens), then builds the Docker image in the background.
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

# ── Scaffold /root/myapp synchronously ─────────────────────────────────────────
# All app files must exist before we mark the lab ready so step 1 works
# immediately when the terminal opens.
log "Scaffolding /root/myapp..."
mkdir -p /root/myapp/chart/templates /root/myapp/bundle

cat > /root/myapp/app.py << 'PYEOF'
from flask import Flask, jsonify, render_template_string
import os

app = Flask(__name__)

HTML = """<!DOCTYPE html>
<html>
<head><title>{{ title }}</title>
<style>
  body { font-family: sans-serif; max-width: 600px; margin: 80px auto; text-align: center; }
  h1 { color: #1a1a2e; }
  p { color: #555; }
  .badge { display: inline-block; background: #e8f4f8; padding: 4px 12px;
           border-radius: 4px; font-size: 14px; margin-top: 8px; }
</style>
</head>
<body>
  <h1>Hello from UDS!</h1>
  <p>This app is running inside a UDS-managed cluster with Istio, Keycloak, and Pepr.</p>
  <p><span class="badge">Environment: {{ env }}</span></p>
</body>
</html>"""

@app.route("/")
def index():
    return render_template_string(HTML,
        title="My UDS App",
        env=os.getenv("APP_ENV", "development"))

@app.route("/health")
def health():
    return jsonify({"status": "ok"})

if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8080)
PYEOF

cat > /root/myapp/requirements.txt << 'EOF'
flask==3.0.3
EOF

cat > /root/myapp/Dockerfile << 'EOF'
FROM python:3.11-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY app.py .
EXPOSE 8080
CMD ["python", "app.py"]
EOF

# values.yaml is configuration — provide it so templates can reference the
# image name and domain without the user writing boilerplate.
cat > /root/myapp/chart/values.yaml << 'EOF'
image: myapp:dev
domain: uds.dev
EOF

log "Scaffold complete — marking lab ready."
touch /var/log/lab-setup/ready

# ── Background: wait for pods, build Docker image ──────────────────────────────
# Zarf picks up images directly from the local Docker daemon (no registry needed).
# A sentinel file at /var/log/lab-setup/image-ready signals the build is done.
# Step 6 waits for this before running uds run dev.
{
  log "Waiting for UDS Core pods..."
  uds zarf tools kubectl wait --for=condition=Available deployment \
    --all --all-namespaces --timeout=300s >> /var/log/lab-setup/uds-setup.log 2>&1 || true
  touch /var/log/lab-setup/pods-ready
  log "UDS Core pods ready."

  log "Building myapp:dev Docker image (may take a few minutes — pulls python:3.11-slim)..."
  docker build -t myapp:dev /root/myapp >> /var/log/lab-setup/uds-setup.log 2>&1
  touch /var/log/lab-setup/image-ready
  log "myapp:dev built — lab fully ready."
} &
