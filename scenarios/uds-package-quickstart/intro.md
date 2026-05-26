# UDS Package Quickstart

You have a containerized app. You want to run it on a UDS cluster — with Istio mesh, automatic NetworkPolicies, and Keycloak SSO available. This scenario shows you the happy path.

## What you'll do

1. Verify a local UDS Core cluster (k3d + Keycloak + Istio + Pepr)
2. Write `zarf.yaml` — the Zarf build manifest
3. Write `values/values.yaml` — the Zarf-variables → Helm bridge
4. Write `manifests/uds-package.yaml` — the UDS Package CR that wires network exposure and policy
5. Build, deploy, and verify the package

## What's happening right now

The environment is installing `k3d` and the `uds` CLI, then deploying **UDS Core** in the background. This takes **5–10 minutes** (mostly image pulls).

You can tail the setup log at any time:

```
tail -f /var/log/lab-setup/uds-setup.log
```

## The sample app

We're packaging **hello-uds** — a minimal nginx deployment. The Helm chart is pre-created at `/root/hello-uds/chart/`. This scenario focuses on the UDS-specific files you have to write, not on the app itself.

```
hello-uds/
├── chart/            ← pre-created Helm chart (nginx)
├── values/           ← you'll create values.yaml here
├── manifests/        ← you'll create uds-package.yaml here
└── zarf.yaml         ← you'll create this
```

When the first step unlocks, UDS Core will be ready.
