#!/bin/bash
# Entrypoint for the lab container — replaces systemd with direct process
# management. Starts: tmux session, ttyd (main + shell), lab-inject.py, and
# optionally the browser stack (Xvfb + Chrome + x11vnc + noVNC).
set -euo pipefail

export HOME=/root

# ── tmux session ──────────────────────────────────────────────────────────────
# Start a named tmux session that ttyd attaches to. The session runs
# lab-entry.sh which waits for the setup readiness flag.
tmux new-session -d -s main '/opt/lab-entry.sh' 2>/dev/null || true

# ── lab-inject.py ─────────────────────────────────────────────────────────────
python3 /opt/lab-inject.py &
INJECT_PID=$!

# ── ttyd terminals ────────────────────────────────────────────────────────────
# :7681 attaches to the shared tmux session (same UX as the VM).
# :7682 provides a direct bash shell (used by the shell proxy).
ttyd --port 7681 --interface 0.0.0.0 --writable \
     tmux attach-session -t main &
TTYD_MAIN_PID=$!

ttyd --port 7682 --interface 0.0.0.0 --writable \
     bash -l &
TTYD_SHELL_PID=$!

# ── Optional browser stack ─────────────────────────────────────────────────────
# Enabled when the LabSession has BrowserEnabled=true, which the pod provider
# sets via BROWSER_ENABLED=true.
if [ "${BROWSER_ENABLED:-false}" = "true" ]; then
  Xvfb :99 -screen 0 1280x900x24 &
  export DISPLAY=:99

  # noVNC websockify bridges :6080 → x11vnc's VNC port (:5900).
  x11vnc -display :99 -nopw -listen 0.0.0.0 -rfbport 5900 \
         -xkb -forever -shared -bg
  websockify --web /usr/share/novnc 6080 localhost:5900 &
fi

# ── Wait for any child to exit ────────────────────────────────────────────────
# If ttyd or lab-inject exits unexpectedly, log it and let the pod restart.
wait -n 2>/dev/null || true
echo "A lab process exited; container will restart." >&2
exit 1
