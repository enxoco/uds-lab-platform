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
  xvfb x11vnc novnc chromium-browser

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

# ── Input injection + verify server ───────────────────────────────────────────
cat > /opt/lab-inject.py << 'EOF'
#!/usr/bin/env python3
import http.server
import subprocess
import json
import os
import socket
import struct
import urllib.request

def cdp_navigate(url):
    targets = json.loads(urllib.request.urlopen('http://127.0.0.1:9222/json').read())
    pages = [t for t in targets if t.get('type') == 'page']
    if not pages:
        return
    ws_url = pages[0]['webSocketDebuggerUrl'].replace('ws://', '')
    host_port, path = ws_url.split('/', 1)
    path = '/' + path
    sock = socket.create_connection(('127.0.0.1', 9222), timeout=3)
    key = __import__('base64').b64encode(os.urandom(16)).decode()
    sock.sendall((
        f'GET {path} HTTP/1.1\r\nHost: 127.0.0.1:9222\r\n'
        f'Upgrade: websocket\r\nConnection: Upgrade\r\n'
        f'Sec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\n\r\n'
    ).encode())
    buf = b''
    while b'\r\n\r\n' not in buf:
        buf += sock.recv(4096)
    msg = json.dumps({'id': 1, 'method': 'Page.navigate', 'params': {'url': url}}).encode()
    mask = os.urandom(4)
    masked = bytes([b ^ mask[i % 4] for i, b in enumerate(msg)])
    n = len(msg)
    header = bytes([0x81, 0x80 | n]) if n < 126 else bytes([0x81, 0xFE]) + struct.pack('>H', n)
    sock.sendall(header + mask + masked)
    sock.close()

class Handler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get('Content-Length', 0))
        try:
            body = json.loads(self.rfile.read(length))
        except Exception:
            body = {}

        if self.path == '/cmd':
            try:
                data = body.get('data', '')
                if data:
                    subprocess.run(['tmux', 'send-keys', '-t', 'main', '-l', data])
                    subprocess.run(['tmux', 'send-keys', '-t', 'main', 'Enter'])
            except Exception:
                pass
            self.send_response(200)
            self.end_headers()

        elif self.path == '/verify':
            step = body.get('step', '')
            script = f'/opt/scenario/verify/step{step}.sh'
            passed = False
            try:
                if os.path.exists(script):
                    result = subprocess.run(
                        ['bash', script],
                        capture_output=True,
                        timeout=30,
                    )
                    passed = result.returncode == 0
            except Exception:
                pass
            response = json.dumps({'pass': passed}).encode()
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Content-Length', str(len(response)))
            self.end_headers()
            self.wfile.write(response)

        elif self.path == '/navigate':
            url = body.get('url', '')
            if url:
                try:
                    cdp_navigate(url)
                except Exception:
                    pass
            self.send_response(200)
            self.end_headers()

        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, *args): pass

http.server.HTTPServer(('0.0.0.0', 7680), Handler).serve_forever()
EOF
chmod +x /opt/lab-inject.py

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
ExecStart=/usr/bin/x11vnc -display :99 -forever -nopw -listen 127.0.0.1 -rfbport 5900 -xkb -noxrecord -noxfixes -noxdamage
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
ExecStart=/usr/bin/chromium-browser \
  --no-sandbox \
  --disable-gpu \
  --disable-dev-shm-usage \
  --no-first-run \
  --kiosk \
  --window-size=1920,1080 \
  --window-position=0,0 \
  --remote-debugging-port=9222 \
  --remote-debugging-address=127.0.0.1 \
  about:blank
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload

# ── Clean up ───────────────────────────────────────────────────────────────────
log "Cleaning up..."
apt-get clean
rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*
log "Base image build complete."
