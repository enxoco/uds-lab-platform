# Step 4 – Deploy with uds run dev

`uds run dev` is the standard development workflow for iterating on a UDS package with its full dependency chain. It reads `tasks.yaml`, builds your Zarf package from local source, creates the bundle defined in `bundle/uds-bundle.yaml`, and deploys it to the running cluster.

## Configure environment-specific overrides

The bundle configures Postgres for production: 2 HA instances, 10Gi volumes, no explicit storage class. This single-node k3d cluster uses the `local-path` provisioner and doesn't need HA.

`uds-config.yaml` is the proper mechanism for environment-specific bundle overrides — prod defaults live in the bundle, deployment-specific values live here. Create it before deploying:

```
cd /root/reference-package
```

```
cat > uds-config.yaml << 'EOF'
packages:
  postgres-operator:
    overrides:
      postgres-operator:
        uds-postgres-config:
          values:
            - path: postgresql
              value:
                enabled: true
                teamId: "uds"
                volume:
                  size: "5Gi"
                  storageClass: "local-path"
                numberOfInstances: 1
                users:
                  reference-package.reference-package: []
                databases:
                  reference: reference-package.reference-package
                version: "15"
                ingress:
                  - remoteNamespace: reference-package
EOF
```

UDS picks this up automatically at deploy time and merges it with the bundle configuration.

> **Why not just edit `bundle/uds-bundle.yaml`?**
> The bundle is the authoritative prod-grade artifact — it should always reflect production defaults. Editing it to fit a dev environment means you're one commit away from shipping under-resourced, single-instance Postgres to production. `uds-config.yaml` keeps environment-specific overrides separate and explicit. You'd commit one per environment (dev, staging, prod) and never touch the bundle itself for per-env tuning.

## Deploy

```
uds run dev
```

This runs three operations in sequence:

1. **Build the Zarf package** — pulls `ghcr.io/uds-packages/reference-package:v0.1.1` into a local OCI cache and packages it with the Helm chart into a `.tar.zst` archive
2. **Create the bundle** — wraps the Zarf package and pulls the `postgres-operator` package from `ghcr.io/uds-packages/postgres-operator`
3. **Deploy the bundle** — deploys `postgres-operator` first (it's listed first in `uds-bundle.yaml`), then `reference-package` on top

> The image cache was pre-warmed in the background when this lab started. If `uds run dev` still takes a few minutes, that's normal — bundle creation and deployment have real work to do.

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
