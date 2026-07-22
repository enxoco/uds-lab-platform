# UDS Lab Platform

Browser-based interactive lab environment for UDS and Zarf. Provisions ephemeral KubeVirt VMs on demand from golden PVC snapshots, serves browser terminals via ttyd, and requires no client installs.

## Architecture

```
Browser → Istio (TLS) → authservice (OIDC) → lab-platform server
                                                    │
                                          lab-platform operator
                                                    │
                                         ┌──────────┴──────────┐
                                         │     uds-lab-vms ns  │
                                    VMI (KubeVirt)              │
                                    DataVolume (CDI clone)      │
                                    NodePort Service            │
                                    NetworkPolicy               │
                                         └─────────────────────┘

Lab VM (boots from golden PVC)
  ├── ttyd :7681   — tmux main session (setup-aware entry)
  ├── ttyd :7682   — direct bash shell
  ├── Python :7680 — lab-inject.py (cmd, verify, navigate, services)
  └── noVNC :6080  — Xvfb + x11vnc + websockify + Chromium (browser: true)
```

### Golden PVCs

VM images are built once with Packer (QEMU/KVM), wrapped in small Python HTTP
server images, and bundled by Zarf. Zarf rewrites the server Pods' normal image
references and supplies registry credentials at deploy time. CDI imports the
qcow2 files from stable cluster-local Services; no Zarf registry address appears
in a DataVolume. Each LabSession then clones the appropriate golden PVC, giving
every user an isolated copy of the full disk image.

| Tier | Golden PVC | Contents |
|------|-----------|----------|
| `base` | `golden-base` | Ubuntu 24.04 + Docker, k3d, uds CLI, neovim, jq, yq, tmux, ttyd, noVNC, Chromium |
| `uds-core` | `golden-uds-core` | Base + k3d-core-slim-dev fully deployed |

## Prerequisites

**Host machine:**
- Bare-metal Linux with `/dev/kvm` (AMD-V or Intel VT-x enabled in BIOS)
- 80+ GB free disk for packer output
- `uds`, `zarf`, `kubectl`, `docker`, `jq`, `ip`, `curl`
- [virtctl](https://kubevirt.io/user-guide/user_workloads/virtctl_client_tool/) (for VM console/SSH access)
- KubeVirt package repo at `~/src/github.com/uds-packages/kubevirt`
- Containerized Data Importer package repo at
  `~/src/github.com/uds-packages/containerized-data-importer`. Until the stacked
  Unicorn flavor PR merges, check out
  `feat/add-unicorn-flavor-update-dependencies`:

  ```bash
  git clone git@github.com:uds-packages/containerized-data-importer.git \
    ~/src/github.com/uds-packages/containerized-data-importer
  git -C ~/src/github.com/uds-packages/containerized-data-importer switch \
    feat/add-unicorn-flavor-update-dependencies
  ```

  `CDI_PKG_DIR` can override this default checkout location.

**First-time only:**
- Internet access (pulls Ubuntu cloud image, packages, UDS Core bundle)

## Quick Start

### Full e2e from scratch

```bash
uds run dev --with CDI_FLAVOR=unicorn
```

This will:
1. Generate a packer SSH keypair (if missing)
2. Build and stage the local unicorn CDI package
3. Wipe and reinstall k3s (MetalLB + KubeVirt + CDI + UDS Core)
4. Build and deploy the lab-platform Docker image
5. Deploy the versioned VM-image package from the UDS Army registry
6. Patch CoreDNS to route `*.uds.dev` to MetalLB gateways
7. Create a test Keycloak user (`doug@uds.dev / unicorn123!@#UN`)
8. Start the nginx SNI proxy for external access

The workflow defaults to `upstream`; the command above explicitly selects the
local `unicorn` flavor. Building the Unicorn flavor requires authentication to
the Defense Unicorns Chainguard registry.

### Build local VM images instead of using the published package

```bash
uds run dev --with BUILD_IMAGES=1 --with LOCAL_VM_IMAGES=1
```

This is only needed when producing a new VM-image package for manual
publication. The normal dev flow uses the package already published at
`registry.uds-mil.us/enxo/lab-vm-images`.

### Keep existing k3s, redeploy platform only

```bash
uds run dev --with WIPE_CLUSTER=0
```

To bypass the registry and use local VM-image archives while keeping the
cluster, run:

```bash
uds run dev --with WIPE_CLUSTER=0 --with BUILD_IMAGES=0 --with LOCAL_VM_IMAGES=1
```

### Rebuild and redeploy operator only (fastest iteration)

```bash
uds run redeploy
```

After the script completes:
- UI: `https://lab.uds.dev`
- Admin: `https://keycloak.admin.uds.dev`
- Test user: `doug@uds.dev / unicorn123!@#UN`

> **Note:** If you redeploy manually (not via `dev.sh`), always re-run
> `./scripts/patch-coredns.sh` afterward — redeploys reset the CoreDNS NodeHosts.

## VM Images (Packer)

Images are built locally with QEMU/KVM and output as qcow2 files in `packer/output/`.

```bash
# Build both images
uds run build-images

# Skip specific tiers (reuse existing qcow2s)
uds run build-images --with skip_base=1
```

Build order: `lab-base` → `playground-uds-core`. The base image includes the
tools previously provided by a separate image. Each stage uses the previous
stage's qcow2 as its base disk. The UDS Core image takes ~45 min
(deploys a full k3d UDS Core cluster inside the VM before snapshotting).

### Import golden PVCs directly from qcow2 files (fallback)

The normal bundle deployment imports from the packaged image-server Services.
Use this host-served path only as a troubleshooting fallback:

```bash
BASE_QCOW2=packer/output/base/lab-base.qcow2 \
UDS_CORE_QCOW2=packer/output/uds-core/lab-playground-uds-core.qcow2 \
./scripts/create-golden-pvc.sh
```

## Available Tasks

```bash
uds run dry-run         # tests + Helm lint/render + Zarf lint/render; no cluster
uds run dev             # full e2e (calls dev.sh)
uds run build-images    # packer builds only
uds run cluster-up      # cluster setup only
uds run patch-coredns   # re-patch CoreDNS after redeploy
uds run create-test-user
uds run start-proxy     # nginx SNI proxy (auto-detects MetalLB IPs)
uds run stop-proxy
uds run redeploy        # rebuild + redeploy operator image, no cluster wipe
```

## VM Access

```bash
# List running VMs
kubectl get vmi -n uds-lab-vms

# Serial console (shows cloud-init / user-data output)
virtctl console <vmi-name> -n uds-lab-vms   # exit: Ctrl+]

# SSH
virtctl ssh --local-ssh-opts="-i $(pwd)/packer/packer-key" \
  lab@vmi/<vmi-name> -n uds-lab-vms
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `VM_NAMESPACE` | `uds-lab-vms` | Namespace for VMIs, DataVolumes, Services |
| `SESSION_TTL_MINUTES` | `60` | Lab session lifetime |
| `PORT` | `8080` | HTTP listen port |
| `SCENARIOS_DIR` | *(embedded)* | Override embedded scenarios with a local directory |
| `STATIC_DIR` | *(embedded)* | Override embedded static files |

## Creating a Scenario

Scenarios live in `scenarios/<id>/`. Each directory needs:

```
scenarios/my-scenario/
├── scenario.yaml
├── setup.sh
├── steps/
│   ├── step1.md
│   └── step2.md
└── verify/           (optional)
    ├── step1.sh
    └── step2.sh
```

### scenario.yaml

```yaml
title: "My Scenario"
description: "What this lab teaches."
duration: 45
difficulty: beginner  # beginner | intermediate | advanced
browser: false        # true = provision Chromium + noVNC
tier: tools           # base | tools | uds-core — selects which golden PVC to clone

steps:
  - title: "Step one"
    text: steps/step1.md
    verify: step1.sh
  - title: "Step two"
    text: steps/step2.md
```

The `tier` field determines which golden PVC is cloned for the session:
- `base` — minimal Ubuntu + terminal tools
- `tools` — base + Docker, k3d, uds CLI
- `uds-core` — tools + a running k3d UDS Core cluster (ready immediately)

**Services** (`services:`) declares named URLs shown as clickable chips in the terminal header:

```yaml
services:
  - label: "SSO (Keycloak)"
    url: "https://sso.uds.dev"
  - label: "Grafana"
    url: "https://grafana.admin.uds.dev"
```

### setup.sh

Runs in the background on the VM after boot. Must touch `/var/log/lab-setup/ready` when complete.

```bash
#!/bin/bash
set -euo pipefail
export HOME=/root

# scenario-specific setup...

touch /var/log/lab-setup/ready
```

For `uds-core` tier scenarios, the k3d cluster is stopped before snapshotting and
must be restarted in `setup.sh`:

```bash
systemctl start docker
k3d cluster start uds
k3d kubeconfig get uds > /root/.kube/config
touch /var/log/lab-setup/ready
```

### Verify scripts

`verify/step<N>.sh` — exit 0 = pass. Run as root on the VM, 30-second timeout.

```bash
#!/bin/bash
export HOME=/root
kubectl get ns my-namespace &>/dev/null
```

### DNS inside the VM

VMs running an inner k3d/k3s cluster need `*.uds.dev` to resolve to `127.0.0.1`
(the inner cluster's ingress), not the outer cluster's MetalLB IPs. This is handled
automatically by dnsmasq in `user-data.sh.gotmpl`:

```
address=/.uds.dev/127.0.0.1   # wildcard — inner cluster
server=1.1.1.1                  # internet DNS
server=8.8.8.8
```

## Development

### Project structure

```
cmd/
  labserver/    # HTTP server: sessions API, WebSocket proxy
  laboperator/  # Kubernetes operator: reconciles LabSession CRDs → VMIs
internal/
  operator/     # operator config, controller
  provider/
    kubevirt/   # KubeVirt provider: VMI + DataVolume + Service + NetworkPolicy
  session/      # session manager, session state
packer/         # QEMU packer builds for each VM tier
chart/          # Helm chart for lab-platform deployment
scripts/        # dev workflow scripts
vm/             # user-data.sh.gotmpl — cloud-init for lab VMs
scenarios/      # lab scenario definitions

~/src/github.com/uds-packages/
  containerized-data-importer/ # External CDI package checkout used by dev
```

### Release process

Release package creation and publishing are currently manual. The release steps
remain disabled in GitHub Actions until the required runner and release
environment are configured.

The VM-image package must be built and published to
`registry.uds-mil.us/enxo/lab-vm-images` before a clean development cluster can
run the default flow. Build it locally with `uds run build-images` followed by
`uds run build-vm-images-package`, then publish it manually with the
`udm-common` publishing tasks.

```bash
# GitHub UI: Actions -> Bump Version -> Run workflow -> select minor/major/patch
# Or via CLI:
gh workflow run bump-version.yaml -f bump_type=minor
```

The UDS bundle (`bundle/uds-bundle.yaml`) depends on `ghcr.io/uds-packages/kubevirt`, a Defense Unicorns internal package. **The bundle must never be built or published from public CI** - it can only be assembled on internal DU infrastructure with access to that registry. The bundle is for local dev use only; `uds run build-bundle` and `uds run deploy-bundle` are not run by any CI workflow.

The KubeVirt package is referenced in the bundle exclusively via its OCI registry URL. No KubeVirt tarballs are ever committed to this repo or produced by CI.

### Iterating on the operator

```bash
# Make code changes, then:
uds run redeploy

# Watch operator logs
kubectl logs -n lab-platform -l app=lab-operator -f

# Create a test session
kubectl apply -f test-session.yaml
kubectl get labsession -A -w
kubectl get vmi -n uds-lab-vms -w
```

### Session Management

Each browser is identified by a `lab_client_id` cookie (HttpOnly, 30-day expiry). Only one active lab session is allowed per client — attempting to start a second returns HTTP 409. The existing session can be ended from the lab UI or by waiting for the TTL to expire.
