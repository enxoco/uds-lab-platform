#!/bin/bash
# Builds the UDS lab base image. Installs all static dependencies so
# user-data only needs to write scenario files and start services.
set -euo pipefail

export HOME=/root
export DEBIAN_FRONTEND=noninteractive

log() { echo "[$(date '+%H:%M:%S')] $*"; }

curl_retry() {
  curl -fsSL \
    --retry 5 \
    --retry-all-errors \
    --retry-delay 3 \
    --connect-timeout 20 \
    --max-time 600 \
    "$@"
}

download_verified() {
  local url="$1" expected_sha256="$2" output="$3"
  curl_retry "$url" -o "$output"
  printf '%s  %s\n' "$expected_sha256" "$output" | sha256sum --check --status || {
    echo "ERROR: SHA-256 verification failed for $url" >&2
    rm -f "$output"
    exit 1
  }
}

verify_gpg_key() {
  local keyfile="$1" expected_fingerprint="$2" actual_fingerprint
  actual_fingerprint=$(gpg --batch --show-keys --with-colons "$keyfile" \
    | awk -F: '$1 == "fpr" { print $10; exit }')
  [ "$actual_fingerprint" = "$expected_fingerprint" ] || {
    echo "ERROR: GPG fingerprint verification failed for $keyfile" >&2
    exit 1
  }
}

# ── Packages ───────────────────────────────────────────────────────────────────
log "Updating and installing packages..."
apt-get update -q
apt-get upgrade -y -q
apt-get install -y -q \
  tmux curl python3 \
  xvfb x11vnc novnc \
  dnsmasq gnupg

# chromium-browser on Ubuntu 24.04 is a snap redirect that hangs in QEMU.
# Use Google Chrome stable (real deb, official apt repo) instead.
log "Installing Google Chrome..."
apt-get install -y -q ca-certificates
install -m 0755 -d /etc/apt/keyrings
GOOGLE_KEY=$(mktemp)
curl_retry https://dl.google.com/linux/linux_signing_key.pub -o "$GOOGLE_KEY"
verify_gpg_key "$GOOGLE_KEY" "EB4C1BFD4F042F6DDDCCEC917721F63BD38B4796"
gpg --dearmor -o /etc/apt/keyrings/google-chrome.gpg "$GOOGLE_KEY"
rm -f "$GOOGLE_KEY"
echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/google-chrome.gpg] https://dl.google.com/linux/chrome/deb/ stable main" \
  > /etc/apt/sources.list.d/google-chrome.list
apt-get update -q
apt-get install -y -q google-chrome-stable

# ── ttyd ───────────────────────────────────────────────────────────────────────
log "Installing ttyd..."
download_verified \
  "https://github.com/tsl0922/ttyd/releases/download/1.7.7/ttyd.x86_64" \
  "8a217c968aba172e0dbf3f34447218dc015bc4d5e59bf51db2f2cd12b7be4f55" \
  /usr/local/bin/ttyd
chmod +x /usr/local/bin/ttyd
log "ttyd: $(ttyd --version 2>&1 | head -1)"

# ── noVNC index ────────────────────────────────────────────────────────────────
ln -sf /usr/share/novnc/vnc_lite.html /usr/share/novnc/index.html

# ── tmux config ────────────────────────────────────────────────────────────────
cat > /root/.tmux.conf << 'EOF'
set -g mouse on
set -g history-limit 10000
EOF

# ── bashrc additions ───────────────────────────────────────────────────────────
cat >> /root/.bashrc << 'EOF'
alias kubectl="uds zarf tools kubectl"
alias k="uds zarf tools kubectl"
if command -v uds &>/dev/null; then
  source <(uds zarf tools kubectl completion bash)
  complete -o default -F __start_kubectl kubectl
  complete -o default -F __start_kubectl k
fi
EOF

# ── Lab entry script ───────────────────────────────────────────────────────────
cat > /opt/lab-entry.sh << 'EOF'
#!/bin/bash
export HOME=/root

if [ ! -f /var/log/lab-setup/ready ]; then
  clear
  echo ""
  echo "  ┌─────────────────────────────────────────────┐"
  echo "  │  Lab environment is being prepared...       │"
  echo "  │  This takes about 1-2 minutes.              │"
  echo "  └─────────────────────────────────────────────┘"
  echo ""

  tail -f /var/log/lab-setup/uds-setup.log &
  TAIL_PID=$!

  while [ ! -f /var/log/lab-setup/ready ]; do
    sleep 3
  done

  kill $TAIL_PID 2>/dev/null
  wait $TAIL_PID 2>/dev/null

  clear
  echo ""
  echo "  ┌─────────────────────────────────────────────┐"
  echo "  │  Lab ready!                                 │"
  echo "  │  Follow the steps in the left panel.        │"
  echo "  └─────────────────────────────────────────────┘"
  echo ""
fi

exec bash -l
EOF
chmod +x /opt/lab-entry.sh

# ── Systemd service units ──────────────────────────────────────────────────────
cat > /etc/systemd/system/ttyd-main.service << 'EOF'
[Unit]
Description=ttyd main terminal (tmux main session)
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/ttyd --port 7681 --interface 0.0.0.0 --writable tmux attach-session -t main
Restart=always
RestartSec=3
Environment=HOME=/root
Environment=TERM=xterm-256color

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/ttyd-shell.service << 'EOF'
[Unit]
Description=ttyd shell terminal (direct bash)
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/ttyd --port 7682 --interface 0.0.0.0 --writable bash -l
Restart=always
RestartSec=3
Environment=HOME=/root
Environment=TERM=xterm-256color

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/lab-inject.service << 'EOF'
[Unit]
Description=Lab input injection server
After=network.target

[Service]
Type=simple
ExecStart=/usr/bin/python3 /opt/lab-inject.py
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/lab-xvfb.service << 'EOF'
[Unit]
Description=Virtual framebuffer
After=network.target

[Service]
Type=simple
ExecStart=/usr/bin/Xvfb :99 -screen 0 1920x1080x24 -ac
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/lab-x11vnc.service << 'EOF'
[Unit]
Description=VNC server on virtual display
After=lab-xvfb.service

[Service]
Type=simple
ExecStart=/usr/bin/x11vnc -display :99 -forever -nopw -listen 127.0.0.1 -rfbport 5900 -xkb -noxrecord -noxdamage
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/lab-novnc.service << 'EOF'
[Unit]
Description=noVNC WebSocket bridge
After=lab-x11vnc.service

[Service]
Type=simple
ExecStart=/usr/bin/websockify --web=/usr/share/novnc 6080 127.0.0.1:5900
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/lab-browser.service << 'EOF'
[Unit]
Description=Chromium in virtual display
After=lab-xvfb.service

[Service]
Type=simple
Environment=DISPLAY=:99
Environment=HOME=/root
ExecStart=/usr/bin/google-chrome-stable \
  --no-sandbox \
  --disable-gpu \
  --disable-dev-shm-usage \
  --no-first-run \
  --no-default-browser-check \
  --disable-sync \
  --disable-extensions \
  --user-data-dir=/tmp/chrome-data \
  --kiosk \
  --window-size=1920,1080 \
  --window-position=0,0 \
  --remote-debugging-port=9222 \
  --remote-debugging-address=127.0.0.1 \
  http://localhost:7681
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# dnsmasq is installed but configured at boot via user-data
systemctl disable dnsmasq

systemctl daemon-reload

# ── Dev tools ─────────────────────────────────────────────────────────────────
log "Installing dev tools..."
apt-get install -y -q neovim jq yamllint

YQ_VERSION=v4.53.2
download_verified \
  "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_amd64" \
  "d56bf5c6819e8e696340c312bd70f849dc1678a7cda9c2ad63eebd906371d56b" \
  /usr/local/bin/yq
chmod +x /usr/local/bin/yq
log "yq: $(yq --version)"

# ── Docker CE (official apt repo) ─────────────────────────────────────────────
log "Installing Docker CE..."
apt-get install -y -q ca-certificates curl
install -m 0755 -d /etc/apt/keyrings
curl_retry https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
verify_gpg_key /etc/apt/keyrings/docker.asc "9DC858229FC7DD38854AE2D88D81803C0EBFCD88"
chmod a+r /etc/apt/keyrings/docker.asc
. /etc/os-release && tee /etc/apt/sources.list.d/docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: ${UBUNTU_CODENAME:-$VERSION_CODENAME}
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/docker.asc
EOF
apt-get update -q
apt-get install -y -q docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
systemctl enable docker
systemctl start docker
log "Docker: $(docker --version)"

# ── k3d ───────────────────────────────────────────────────────────────────────
log "Installing k3d..."
K3D_VERSION=v5.9.0
download_verified \
  "https://github.com/k3d-io/k3d/releases/download/${K3D_VERSION}/k3d-linux-amd64" \
  "06d8f25bc3a971c4eb29e0ff08429b180402db0f4dec838c9eac427e296800a0" \
  /usr/local/bin/k3d
chmod +x /usr/local/bin/k3d
log "k3d: $(k3d version)"

# ── uds CLI ───────────────────────────────────────────────────────────────────
log "Installing uds CLI..."
UDS_VERSION=v0.33.0
UDS_RELEASE="$(mktemp)"
curl_retry \
  -H 'Accept: application/vnd.github+json' \
  "https://api.github.com/repos/defenseunicorns/uds-cli/releases/tags/${UDS_VERSION}" \
  -o "$UDS_RELEASE"
UDS_ASSET=$(jq -r '.assets[] | select(.name | endswith("_Linux_amd64")) | .name' "$UDS_RELEASE" | head -1)
UDS_SHA256=$(jq -r --arg asset "$UDS_ASSET" \
  '.assets[] | select(.name == $asset) | .digest' "$UDS_RELEASE" | sed 's/^sha256://')
[ -n "$UDS_ASSET" ] || { echo "ERROR: no Linux amd64 asset found for UDS CLI ${UDS_VERSION}" >&2; exit 1; }
[[ "$UDS_SHA256" =~ ^[0-9a-fA-F]{64}$ ]] || { echo "ERROR: no valid checksum found for $UDS_ASSET" >&2; exit 1; }
download_verified \
  "https://github.com/defenseunicorns/uds-cli/releases/download/${UDS_VERSION}/${UDS_ASSET}" \
  "$UDS_SHA256" \
  /usr/local/bin/uds
rm -f "$UDS_RELEASE"
chmod +x /usr/local/bin/uds
log "uds: $(uds version)"

# ── Clean up and zero free space ──────────────────────────────────────────────
log "Cleaning up..."
apt-get clean
rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*
# Zero free blocks so qemu-img convert can skip them when compacting.
# This is NOT compression — it just makes the sparse qcow2 smaller on disk.
log "Trimming free space..."
fstrim -v / || true
log "Base image build complete."
