# Step 4 – Deploy with uds run dev

`uds run dev` is the standard development workflow for iterating on a UDS package with its full dependency chain. It reads `tasks.yaml`, builds your Zarf package from local source, creates the bundle defined in `bundle/uds-bundle.yaml`, and deploys it to the running cluster.

## Deploy

```
cd /root/reference-package && uds run dev
```

This runs three operations in sequence:

1. **Build the Zarf package** — pulls `ghcr.io/uds-packages/reference-package:v0.1.1` into a local OCI cache and packages it with the Helm chart into a `.tar.zst` archive
2. **Create the bundle** — wraps the Zarf package and pulls the `postgres-operator` package from `ghcr.io/uds-packages/postgres-operator`
3. **Deploy the bundle** — deploys `postgres-operator` first (it's listed first in `uds-bundle.yaml`), then `reference-package` on top

> The image cache was pre-warmed in the background when this lab started. If `uds run dev` still takes a few minutes, that's normal — bundle creation and deployment have real work to do.

## Patch Postgres for the lab environment

The bundle configures Postgres for production: 2 HA instances, 10Gi volumes, no explicit storage class. This single-node k3d cluster uses the `local-path` provisioner and doesn't need HA. Patch the running PostgreSQL cluster to match:

```
uds zarf tools kubectl patch postgresql \
  -n postgres \
  $(uds zarf tools kubectl get postgresql -n postgres -o jsonpath='{.items[0].metadata.name}') \
  --type=merge \
  -p '{"spec":{"numberOfInstances":1,"volume":{"size":"5Gi","storageClass":"local-path"}}}'
```

The Zalando operator reconciles the change immediately — it deletes the pending StatefulSet and recreates it with a PVC that `local-path` can provision.

This is the same pattern operators use in any environment: the bundle captures production-grade defaults, and `kubectl patch` applies environment-specific overrides to the running resource without touching the package or bundle source.

## Watch the rollout

Open a Shell Terminal (Tab 2) and monitor progress:

```
watch uds zarf tools kubectl get pods -A
```

You're waiting for pods in `postgres` and `reference-package` namespaces to reach `Running`.

## Check the Package CR

Once deploy completes, Pepr begins reconciling the Package CR:

```
uds zarf tools kubectl get package -n reference-package -w
```

Hit `Ctrl-C` when you see `Phase: Ready`.

## Verify

```
uds zarf tools kubectl get pods -n reference-package --no-headers | awk '$3=="Running"' | wc -l
```

At least one pod running in `reference-package` namespace.
