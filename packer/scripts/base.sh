#!/bin/bash
# Builds the UDS lab base image. Installs all static dependencies so
# user-data only needs to write scenario files and start services.
set -euo pipefail

export HOME=/root
export DEBIAN_FRONTEND=noninteractive

log() { echo "[$(date '+%H:%M:%S')] $*"; }

# ── Packages ───────────────────────────────────────────────────────────────────
log "Updating and installing packages..."
apt-get update -q
apt-get upgrade -y -q
apt-get install -y -q \
  tmux curl python3 \
  xvfb x11vnc novnc \
  dnsmasq

# chromium-browser on Ubuntu 24.04 is a snap redirect that hangs in QEMU.
# Use Google Chrome stable (real deb, official apt repo) instead.
log "Installing Google Chrome..."
apt-get install -y -q ca-certificates
curl -fsSL https://dl.google.com/linux/linux_signing_key.pub \
  | gpg --dearmor -o /etc/apt/keyrings/google-chrome.gpg
echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/google-chrome.gpg] https://dl.google.com/linux/chrome/deb/ stable main" \
  > /etc/apt/sources.list.d/google-chrome.list
apt-get update -q
apt-get install -y -q google-chrome-stable

# ── ttyd ───────────────────────────────────────────────────────────────────────
log "Installing ttyd..."
curl -sSL \
  "https://github.com/tsl0922/ttyd/releases/download/1.7.7/ttyd.x86_64" \
  -o /usr/local/bin/ttyd
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

# ── Clean up ───────────────────────────────────────────────────────────────────
log "Cleaning up..."
apt-get clean
rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*
log "Base image build complete."
