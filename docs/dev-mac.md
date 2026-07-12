# Local Development (Pod Provider — macOS / no nested-virt)

KubeVirt requires KVM which macOS cannot expose. The **pod provider** runs each
lab session as a Kubernetes Pod instead of a VM — no nested virtualisation needed.

The operator can run outside the cluster (it only talks to the k8s API). The
lab server **must** run inside the cluster so it can resolve session service DNS
(`lab-xxx.uds-lab-vms.svc.cluster.local`) when proxying terminal/browser traffic.

## Prerequisites

- Docker (Desktop, OrbStack, or Rancher Desktop) — running
- `k3d` — `brew install k3d`
- `kubectl` — `brew install kubectl`
- Go 1.22+

## Quick start

One script handles the full reset:

```bash
./scripts/pod-dev.sh
```

It deletes and recreates the k3d cluster, installs the CRD, builds both
images, deploys the lab server, and starts a port-forward on :8080.

**Options:**

| Flag | Effect |
|---|---|
| `--skip-wipe` | Keep the cluster, just rebuild images + redeploy |
| `--skip-images` | Full wipe + redeploy, skip image rebuilds |

## Run the operator (separate terminal)

While `pod-dev.sh` is running, open a second terminal:

```bash
PROVIDER_TYPE=pod \
LAB_IMAGE=ghcr.io/enxoco/uds-lab:dev \
VM_NAMESPACE=uds-lab-vms \
SERVER_NAMESPACE=uds-lab-platform \
  go run ./cmd/laboperator/
```

Open **http://localhost:8080** in your browser.

The operator's metrics server binds on `:8081`. Override with
`METRICS_ADDR=:9090` or disable with `METRICS_ADDR=0`.

## Dev workflow summary

| Terminal | Command |
|---|---|
| 1 | `./scripts/pod-dev.sh` (starts port-forward at the end) |
| 2 | `PROVIDER_TYPE=pod LAB_IMAGE=ghcr.io/enxoco/uds-lab:dev go run ./cmd/laboperator/` |
| Browser | `http://localhost:8080` |

## Iterating on the lab server

```bash
./scripts/pod-dev.sh --skip-wipe
```

Rebuilds both images, deletes the labserver pod, redeploys, and restarts
the port-forward — cluster and existing session pods are untouched.

## Iterating on the lab container image

```bash
./scripts/pod-dev.sh --skip-wipe
# or just the image step:
docker build --platform linux/amd64 -t ghcr.io/enxoco/uds-lab:dev -f docker/lab/Dockerfile . \
  && k3d image import ghcr.io/enxoco/uds-lab:dev -c uds-lab
# New sessions will use the updated image; existing pods are unaffected
```

## Manual setup (step by step)

If you prefer to run steps individually:

```bash
# Cluster
k3d cluster delete uds-lab 2>/dev/null || true
k3d cluster create uds-lab --agents 0 --no-lb

# Namespaces + CRD
kubectl create namespace uds-lab-vms
kubectl create namespace uds-lab-platform
kubectl apply -f deploy/crd/

# Images
docker build --platform linux/amd64 -t ghcr.io/enxoco/uds-lab:dev -f docker/lab/Dockerfile .
docker build --platform linux/amd64 -t ghcr.io/enxoco/uds-labserver:dev -f docker/labserver/Dockerfile .
k3d image import ghcr.io/enxoco/uds-lab:dev ghcr.io/enxoco/uds-labserver:dev -c uds-lab

# Deploy + forward
kubectl apply -f deploy/dev/labserver.yaml
kubectl port-forward pod/labserver -n uds-lab-platform 8080:8080
```

## Tear down

```bash
k3d cluster delete uds-lab
```

## Notes

- **Pause/resume**: not supported by the pod backend — users see an error if they
  try, which is expected for local dev.
- **Auth**: without Istio/authservice, `X-Auth-Request-Email` is not injected.
  Sessions are created with an empty email and identified by a browser cookie.
  This is fine for feature testing; the CSM dashboard will show empty emails.
- **Scenarios**: the server uses the embedded scenario FS by default. To iterate
  on scenario content without rebuilding the image, set `SCENARIOS_DIR` in the
  pod's env to a path mounted via a ConfigMap or hostPath volume.
- **OrbStack users**: if you already have OrbStack's built-in k8s enabled, skip
  `k3d cluster create` and apply the CRD + namespaces against the OrbStack
  context directly.
