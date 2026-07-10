# Step 4 – Deploy with uds run dev

`uds run dev` is the standard development workflow for iterating on a UDS package with its full dependency chain. It reads `tasks.yaml`, builds your Zarf package from local source, creates the bundle defined in `bundle/uds-bundle.yaml`, and deploys it to the running cluster.

## About the bundle configuration

The upstream `bundle/uds-bundle.yaml` configures Postgres for production: 2 HA instances, 10Gi volumes. This single-node k3d cluster doesn't need HA and has limited storage. The lab pre-applied a configuration patch to right-size it:

- `numberOfInstances: 1` (single Postgres instance)
- `volume.size: 5Gi`
- `volume.storageClass: local-path` (the k3d built-in provisioner)
- Reduced CPU/memory resource requests

This is intentional — a well-structured bundle exposes these as named `variables` with defaults that operators can override at deploy time without touching the file. The upstream bundle uses `values` blocks instead, so patching is the only option here. That's a pattern to avoid when authoring your own bundles.

## Deploy

```
cd /root/reference-package
```

```
uds run dev
```

This runs three operations in sequence:

1. **Build the Zarf package** — pulls `ghcr.io/uds-packages/reference-package:v0.1.1` into a local OCI cache and packages it with the Helm chart into a `.tar.zst` archive
2. **Create the bundle** — wraps the Zarf package and pulls the `postgres-operator` package from `ghcr.io/uds-packages/postgres-operator`
3. **Deploy the bundle** — deploys `postgres-operator` first (listed first in `uds-bundle.yaml`), then `reference-package` on top of it

> The image cache was pre-warmed when this lab started. If `uds run dev` still takes a few minutes, that's normal — bundle creation and deployment have real work to do.

## Watch the rollout

Open a Shell Terminal (Tab 2) and monitor:

```
watch uds zarf tools kubectl get pods -A
```

Wait for pods in the `postgres` and `reference-package` namespaces to reach `Running`.

## Check the Package CR

Once deploy completes, Pepr begins reconciling the Package CR:

```
uds zarf tools kubectl get package -n reference-package -w
```

Hit `Ctrl-C` when Phase reaches `Ready`.

## Verify

```
uds zarf tools kubectl get pods -n reference-package --no-headers | awk '$3=="Running"' | wc -l
```

At least one pod running in the `reference-package` namespace.
