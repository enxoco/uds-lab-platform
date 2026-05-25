# UDS Lab Platform

Browser-based interactive lab environment for UDS and Zarf. Provisions ephemeral Hetzner VMs on demand, serves browser terminals via ttyd, and requires no client installs.

## Architecture

```
Browser
  │
  ├── GET /                         → catalog (index.html)
  ├── GET /lab.html                 → lab UI (instructions + terminal)
  ├── POST /api/sessions            → provision Hetzner VM
  ├── GET  /api/sessions/{id}       → poll VM status
  ├── DELETE /api/sessions/{id}     → destroy VM
  ├── /t/{id}/                      → WebSocket proxy → VM:7681 (ttyd + tmux)
  ├── /t/{id}/shell/                → WebSocket proxy → VM:7682 (ttyd direct bash)
  ├── /t/{id}/cmd POST              → HTTP proxy → VM:7680/cmd (tmux inject)
  ├── /api/sessions/{id}/verify/{n} → VM:7680/verify (run verify script)
  └── /vnc/{id}/                    → WebSocket proxy → VM:6080 (noVNC/websockify)

Hetzner VM (boots from pre-built snapshot)
  ├── ttyd :7681   — tmux main session (setup-aware entry)
  ├── ttyd :7682   — direct root bash
  ├── Python :7680 — lab-inject.py (cmd + verify endpoints)
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

Builds three images in order and automatically updates `image:` in the playground scenario.yaml files:

| Image | Contents |
|-------|----------|
| `uds-lab-base` | Ubuntu 24.04 + tmux, ttyd, noVNC, Chromium, Python inject server, systemd units |
| `uds-lab-playground-tools` | Base + Docker, k3d, uds CLI, neovim, jq, yq, yamllint |
| `uds-lab-playground-uds-core` | Tools + k3d-core-slim-dev fully deployed |

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
playground: false     # true = shows Playground badge in catalog
image: ""             # optional: snapshot name, overrides VM_IMAGE default

steps:
  - title: "Step one"
    text: steps/step1.md
    verify: step1.sh   # omit if no verification for this step
  - title: "Step two"
    text: steps/step2.md
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

Standard markdown rendered in the instructions panel. Code blocks are clickable — clicking sends the block to the tmux session via the injection server. The "⬡ Browser" button appears in the terminal header when `browser: true`.

### Verify scripts

`verify/step<N>.sh` — `<N>` is 1-indexed, matching step order. Exit 0 = pass, non-zero = fail. Scripts run as root on the VM with a 30-second timeout. Steps with a verify script show a **Check** button that must pass before **Next** is enabled.

```sh
#!/bin/bash
export HOME=/root   # required — scripts run non-interactively via systemd

uds zarf tools kubectl get ns my-namespace &>/dev/null
```

## Creating a Playground Scenario

Playground scenarios boot from a pre-provisioned snapshot so the VM is ready immediately. Set `playground: true` and `image:` to the snapshot name. The `build-images.sh` script sets `image:` automatically.

For snapshots containing a running k3d cluster, `setup.sh` needs to restart Docker and wait for the cluster to come back up:

```sh
#!/bin/bash
set -euo pipefail
export HOME=/root
mkdir -p /var/log/lab-setup

systemctl start docker

for i in $(seq 1 60); do
  uds zarf tools kubectl get nodes --request-timeout=5s &>/dev/null && break
  sleep 5
done

touch /var/log/lab-setup/ready
```
