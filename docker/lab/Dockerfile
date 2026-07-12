# Lab container image — the pod provider backend for local / macOS development.
#
# Mirrors the software installed by packer/scripts/base.sh but replaces systemd
# with a simple process-supervisor entrypoint. Scenario setup.sh runs in the
# pod's init-container (see internal/provider/pod/provider.go); this image only
# needs to provide the persistent services: ttyd + lab-inject.py (+ optional
# browser stack when BROWSER_ENABLED=true).
#
# Build (from repo root):
#   ./scripts/pod-dev.sh --skip-wipe
#   # or manually:
#   docker build -t ghcr.io/enxoco/uds-lab:dev -f docker/lab/Dockerfile .
#
# Run locally (smoke-test):
#   docker run --rm -p 7681:7681 -p 7680:7680 ghcr.io/enxoco/uds-lab:dev

FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive
ENV HOME=/root
ENV TERM=xterm-256color

# ── Base packages ─────────────────────────────────────────────────────────────
RUN apt-get update -q && \
    apt-get install -y -q --no-install-recommends \
      tmux curl python3 ca-certificates \
      xvfb x11vnc novnc \
      dnsmasq \
      tini \
    && rm -rf /var/lib/apt/lists/*

# ── ttyd ──────────────────────────────────────────────────────────────────────
# Use the same version as the VM image for behavioural parity.
ARG TTYD_VERSION=1.7.7
RUN ARCH="$(uname -m)" && \
    curl -fsSL \
      "https://github.com/tsl0922/ttyd/releases/download/${TTYD_VERSION}/ttyd.${ARCH}" \
      -o /usr/local/bin/ttyd && \
    chmod +x /usr/local/bin/ttyd

# ── noVNC ─────────────────────────────────────────────────────────────────────
RUN ln -sf /usr/share/novnc/vnc_lite.html /usr/share/novnc/index.html

# ── tmux config ───────────────────────────────────────────────────────────────
RUN cat > /root/.tmux.conf << 'EOF'
set -g mouse on
set -g history-limit 10000
EOF

# ── lab-inject.py ─────────────────────────────────────────────────────────────
COPY vm/lab-inject.py /opt/lab-inject.py

# ── Scenarios ─────────────────────────────────────────────────────────────────
# The pod provider's init-container runs setup.sh from inside the container.
# Bake all scenarios in so any scenario ID works without a ConfigMap mount.
RUN mkdir -p /lab-work /var/log/lab-setup
COPY scenarios/ /lab/scenarios/
RUN find /lab/scenarios -name "*.sh" -exec chmod +x {} \;

# ── Lab entry script (mirrors /opt/lab-entry.sh in the VM) ───────────────────
RUN cat > /opt/lab-entry.sh << 'ENTRY'
#!/bin/bash
export HOME=/root

if [ ! -f /var/log/lab-setup/ready ]; then
  clear
  echo ""
  echo "  ┌─────────────────────────────────────────────┐"
  echo "  │  Lab environment is being prepared...       │"
  echo "  └─────────────────────────────────────────────┘"
  echo ""
  while [ ! -f /var/log/lab-setup/ready ]; do sleep 2; done
  clear
fi

exec bash -l
ENTRY
RUN chmod +x /opt/lab-entry.sh

# ── Entrypoint ────────────────────────────────────────────────────────────────
# tini reaps zombie processes (important when tmux and ttyd both fork children).
COPY docker/lab/entrypoint.sh /opt/entrypoint.sh
RUN chmod +x /opt/entrypoint.sh

EXPOSE 7680 7681 7682 6080

ENTRYPOINT ["/usr/bin/tini", "--", "/opt/entrypoint.sh"]
