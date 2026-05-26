# Base Image Rebuild Guide

Changes fall into two categories: those that take effect immediately on the next server deploy, and those that require rebuilding VM images via Packer.

## Does my change require a rebuild?

### No rebuild needed (server-side only)

These changes take effect when the Go binary is redeployed:

- **Frontend** (`web/`) — HTML, CSS, JavaScript changes
- **Scenario content** (`scenarios/`) — step markdown, `scenario.yaml`, setup scripts, verify scripts
- **Server logic** (`cmd/`, `internal/`) — API behavior, session management, proxy routing
- **VM user-data template** (`vm/user-data.sh.gotmpl`) — rendered fresh on every session creation
- **Lab inject server** (`vm/lab-inject.py`) — embedded in user-data, rendered fresh each session
- **noVNC viewer switch** (e.g., `vnc_lite.html` → `vnc.html`) — URL is constructed server-side

### Rebuild required (Packer)

These changes are baked into the VM snapshot and will not affect existing images:

| What changed | Which image(s) to rebuild |
|---|---|
| System packages (`apt install`) | Base → all downstream |
| systemd service units (ttyd, x11vnc, noVNC, lab-browser, lab-inject, lab-xvfb) | Base → all downstream |
| x11vnc flags (e.g. clipboard, display options) | Base → all downstream |
| `/opt/lab-entry.sh` (tmux session entry script) | Base → all downstream |
| ttyd binary or version | Base → all downstream |
| Chromium version or config | Base → all downstream |
| Docker, k3d, UDS CLI versions | Tools playground → UDS Core playground |
| UDS Core bundle or version | UDS Core playground only |
| k3d cluster config | UDS Core playground only |

**Rule of thumb:** if the change affects anything that runs before `user-data.sh` executes, it needs a rebuild.

## How to rebuild

```bash
export HCLOUD_TOKEN=<your-token>
cd packer/

# Rebuild everything (base → tools → uds-core)
./build-images.sh

# Skip unchanged layers
SKIP_BASE=1 BASE_IMAGE=<existing-snapshot-name> ./build-images.sh
SKIP_BASE=1 SKIP_TOOLS=1 BASE_IMAGE=<name> TOOLS_IMAGE=<name> ./build-images.sh
```

Snapshots are auto-discovered by the server at session creation via Hetzner label selectors — no config changes needed after a rebuild.

## Image dependency chain

```
lab-base  ──→  lab-playground-tools  ──→  lab-playground-uds-core
```

Rebuilding a layer requires rebuilding all downstream layers too. Base rebuilds cascade to both playground tiers (~15 minutes total for a full rebuild).

## Current known rebuild-required changes

- `packer/scripts/base.sh`: removed `-noxfixes` from x11vnc to enable clipboard sync (see [issue #1](https://github.com/enxoco/uds-lab-platform/issues/1))
