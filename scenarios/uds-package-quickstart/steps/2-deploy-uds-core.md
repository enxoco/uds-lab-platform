# Step 2 – Deploy UDS Core

UDS Core is the foundation layer: a k3d cluster pre-loaded with Istio, Keycloak, and the Pepr operator. One command provisions everything.

## Deploy

```
uds deploy k3d-core-slim-dev:latest --confirm
```

This pulls and deploys:
- **k3d** — Kubernetes cluster in Docker
- **Istio** — service mesh (control plane + ingress gateways)
- **Keycloak** — SSO and identity provider
- **Pepr** — the UDS policy operator that watches `Package` CRs

> **This takes 5–10 minutes.** The terminal will stream progress. Grab a coffee.

## What happens under the hood

UDS CLI calls Zarf to unpack the bundle, creates a k3d cluster, pushes images into the in-cluster registry, and deploys Helm charts in dependency order.

The resulting cluster runs entirely on this VM — no external Kubernetes required.

## Verify the cluster

Once the command returns, check that everything is running:

```
uds zarf tools kubectl get pods -A
```

```
uds zarf tools kubectl get namespaces | grep -E 'uds|istio|keycloak'
```

| Namespace | What lives there |
|---|---|
| `uds-dev-stack` | Pepr operator, monitoring |
| `istio-system` | Istiod control plane |
| `keycloak` | Keycloak SSO |

## `*.uds.dev` DNS

`uds.dev` is a public wildcard DNS entry that resolves to `127.0.0.1`. So `hello-uds.uds.dev` will hit your local cluster without any `/etc/hosts` editing.

## Verify

When all pods across `uds-dev-stack`, `istio-system`, and `keycloak` are `Running`, click **Next** to start building your package.
