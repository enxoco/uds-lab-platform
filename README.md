# UDS Lab Platform

Browser-based interactive lab environment for UDS and Zarf. Provisions ephemeral Hetzner VMs on demand, serves browser terminals via ttyd, and requires no client installs.

## Architecture

```
Browser
  │
  ├── GET /                          → catalog (index.html)
  ├── GET /lab.html                  → lab UI (instructions + terminal)
  ├── POST /api/sessions             → provision Hetzner VM
  ├── GET  /api/sessions/{id}        → poll VM status
  ├── DELETE /api/sessions/{id}      → destroy VM
  ├── POST /api/sessions/{id}/verify/{n} → run verify script on VM
  ├── /t/{id}/                       → WebSocket proxy → VM:7681 (ttyd + tmux)
  ├── /t/{id}/shell/                 → WebSocket proxy → VM:7682 (ttyd direct bash)
  ├── /t/{id}/cmd POST               → HTTP proxy → VM:7680/cmd (tmux inject)
  ├── /t/{id}/navigate POST          → HTTP proxy → VM:7680/navigate (VM browser)
  ├── GET /api/sessions/{id}/services → scenario services + VM auto-detect
  └── /vnc/{id}/                     → WebSocket proxy → VM:6080 (noVNC/websockify)

Hetzner VM (boots from pre-built snapshot)
  ├── ttyd :7681   — tmux main session (setup-aware entry)
  ├── ttyd :7682   — direct root bash
  ├── Python :7680 — lab-inject.py (cmd, verify, navigate, services endpoints)
  └── noVNC :6080  — Xvfb + x11vnc + websockify + Chromium (browser: true only)
```

## Prerequisites

- Go 1.22+
- [Packer](https://developer.hashicorp.com/packer/install) (for building VM images)
- `jq` (for the build script)
- Hetzner Cloud account + API token
- SSH key named `local` in your Hetzner project

## Quick Start

### 1. Build the base VM image

```sh
cd packer
packer init lab-base.pkr.hcl
HCLOUD_TOKEN=<token> packer build lab-base.pkr.hcl
```

The snapshot name is printed at the end and written to `packer/manifest.json`.

### 2. Run the server

```sh
# Use pre-built base image (recommended)
VM_IMAGE=uds-lab-base-20260101-120000 HCLOUD_TOKEN=<token> go run ./cmd/labserver

# Falls back to ubuntu-24.04 if VM_IMAGE is unset (slower — installs packages at boot)
HCLOUD_TOKEN=<token> go run ./cmd/labserver
```

The server prompts interactively for `HCLOUD_TOKEN` if not set as an env var.

### 3. Open http://localhost:8080

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `HCLOUD_TOKEN` | *(prompted)* | Hetzner Cloud API token |
| `VM_IMAGE` | `ubuntu-24.04` | Default image/snapshot name for new VMs |
| `VM_SERVER_TYPE` | `ccx13` | Hetzner server type |
| `VM_LOCATION` | `hil` | Hetzner datacenter location |
| `SESSION_TTL_MINUTES` | `60` | Lab session lifetime in minutes |
| `PORT` | `8080` | HTTP listen port |
| `SCENARIOS_DIR` | *(embedded)* | Override embedded scenarios with a local directory |
| `STATIC_DIR` | *(embedded)* | Override embedded static files with a local directory |

## Building VM Images

VM images are built with [Packer](https://developer.hashicorp.com/packer) using the Hetzner Cloud plugin. Each image layer builds on the previous one.

### All images (recommended)

```sh
cd packer
HCLOUD_TOKEN=<token> ./build-images.sh
```

Builds three images in order. Playground snapshots are tagged with Hetzner labels and auto-discovered at session creation — no manual config needed.

| Image | Labels | Contents |
|-------|--------|----------|
| `uds-lab-base` | `role=uds-lab-base` | Ubuntu 24.04 + tmux, ttyd, noVNC, Chromium, Python inject server, systemd units |
| `uds-lab-playground-tools` | `role=uds-lab-playground,tier=tools` | Base + Docker, k3d, uds CLI, neovim, jq, yq, yamllint |
| `uds-lab-playground-uds-core` | `role=uds-lab-playground,tier=uds-core` | Tools + k3d-core-slim-dev fully deployed |

The UDS Core image build takes ~15 minutes.

### Partial rebuilds

```sh
# Rebuild only playgrounds, skip base
SKIP_BASE=1 BASE_IMAGE=uds-lab-base-20260101-120000 \
  HCLOUD_TOKEN=<token> ./build-images.sh

# Rebuild only uds-core playground, skip base + tools
SKIP_BASE=1 SKIP_TOOLS=1 \
  BASE_IMAGE=uds-lab-base-20260101-120000 \
  TOOLS_IMAGE=uds-lab-playground-tools-20260101-120000 \
  HCLOUD_TOKEN=<token> ./build-images.sh

# Rebuild only uds-core playground (skip base + tools)
SKIP_BASE=1 SKIP_TOOLS=1 SKIP_UDS_CORE=0 \
  BASE_IMAGE=... TOOLS_IMAGE=... \
  HCLOUD_TOKEN=<token> ./build-images.sh
```

## Binary Distribution

The server embeds all static files, scenarios, and the user-data template at build time:

```sh
go build -o labserver ./cmd/labserver
./labserver   # prompts for HCLOUD_TOKEN
```

Override embedded assets for local development:

```sh
SCENARIOS_DIR=./scenarios STATIC_DIR=./web/static go run ./cmd/labserver
```

## Creating a Scenario

Scenarios live in `scenarios/<id>/`. Each directory needs:

```
scenarios/my-scenario/
├── scenario.yaml
├── setup.sh
├── steps/
│   ├── step1.md
│   └── step2.md
└── verify/           (optional — one file per verified step)
    ├── step1.sh
    └── step2.sh
```

### scenario.yaml

```yaml
title: "My Scenario"
description: "What this lab teaches."
duration: 45          # minutes shown in catalog
difficulty: beginner  # beginner | intermediate | advanced
browser: false        # true = provision Chromium + noVNC on the VM
playground: false     # true = shows Playground badge; image auto-discovered by label
image: ""             # optional: snapshot name/ID, overrides VM_IMAGE env var

steps:
  - title: "Step one"
    text: steps/step1.md
    verify: step1.sh   # omit if no verification for this step
  - title: "Step two"
    text: steps/step2.md
```

Playground scenarios (`playground: true`) do not need `image:` — the server queries Hetzner for the most recent snapshot with labels `role=uds-lab-playground,tier=<scenario-suffix>`. For example, scenario `playground-uds-core` looks for `tier=uds-core`.

**Services** (`services:`) declares named URLs that appear as clickable chips in the terminal header and a collapsible panel in the instructions sidebar. Clicking any service chip opens it in the VM browser (requires `browser: true`). Scenario-defined services are merged with any URLs auto-detected from the VM at `/api/sessions/{id}/services`.

```yaml
services:
  - label: "SSO (Keycloak)"
    url: "https://sso.uds.dev"
  - label: "Grafana"
    url: "https://grafana.admin.uds.dev"
```

### setup.sh

Runs in the background on the VM after boot. Must touch `/var/log/lab-setup/ready` when complete — the terminal entry screen waits for this file before showing the prompt.

```sh
#!/bin/bash
set -euo pipefail
export HOME=/root

# install/configure scenario dependencies...

touch /var/log/lab-setup/ready
```

### Step markdown

Standard markdown rendered in the instructions panel.

**Click-to-run code blocks** — clicking any fenced code block sends it to the tmux session via the injection server.

**VNC link interception** — any link to a `*.uds.dev` hostname is tagged with a ⬡ indicator. Clicking it automatically opens the VM browser (noVNC) and navigates to that URL inside the VM. Links must use explicit markdown syntax (`[https://sso.uds.dev](https://sso.uds.dev)`) — bare URLs in tables are not autolinked.

The **⬡ Browser** button in the terminal header opens the noVNC window manually. Only visible when `browser: true`.

### Verify scripts

`verify/step<N>.sh` — `<N>` is 1-indexed, matching step order. Exit 0 = pass, non-zero = fail. Scripts run as root on the VM with a 30-second timeout. Steps with a verify script show a **Check** button that must pass before **Next** is enabled.

```sh
#!/bin/bash
export HOME=/root   # required — scripts run non-interactively via systemd

uds zarf tools kubectl get ns my-namespace &>/dev/null
```

## Creating a Playground Scenario

Playground scenarios boot from a pre-provisioned snapshot so the environment is ready immediately. Set `playground: true` — the snapshot is auto-discovered via Hetzner labels (see [Building VM Images](#building-vm-images)). No `image:` needed.

For snapshots with a k3d cluster, the cluster is stopped cleanly before snapshotting and must be restarted in `setup.sh`. Mark the lab ready as soon as the cluster is up, then wait for pods in the background so users get terminal access immediately:

```sh
#!/bin/bash
set -euo pipefail
export HOME=/root
mkdir -p /var/log/lab-setup /root/.kube

systemctl start docker
k3d cluster start uds
k3d kubeconfig get uds > /root/.kube/config
chmod 600 /root/.kube/config

# Mark ready immediately — give terminal access while pods finish starting
touch /var/log/lab-setup/ready

# Wait for workloads in background; remove init notice when done
{
  uds zarf tools kubectl wait --for=condition=Available deployment \
    --all --all-namespaces --timeout=300s 2>/dev/null || true
  uds zarf tools kubectl wait --for=condition=Ready statefulset \
    --all --all-namespaces --timeout=300s 2>/dev/null || true
  touch /var/log/lab-setup/pods-ready
} &
```

Add a `/etc/profile.d/` script to show an initialization notice until pods are ready:

```sh
# In setup.sh, before touching ready:
cat > /etc/profile.d/lab-init-status.sh << 'EOF'
if [ ! -f /var/log/lab-setup/pods-ready ]; then
  echo ""
  echo "  ⚡ Pods are still initializing. Monitor: uds zarf tools monitor"
  echo ""
fi
EOF
```

## Session Management

Each browser is identified by a `lab_client_id` cookie (HttpOnly, 30-day expiry). Only one active lab session is allowed per client — attempting to start a second returns HTTP 409 with a user-friendly message. The existing session can be ended from the lab UI or by waiting for the TTL to expire.

This is a placeholder for future GitHub OAuth integration. When auth is added, the cookie-based client ID in `clientID()` (`cmd/labserver/main.go`) will be replaced with the authenticated GitHub user ID — no other changes required.
