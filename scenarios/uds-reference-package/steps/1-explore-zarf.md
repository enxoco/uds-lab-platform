# Step 1 – Explore the reference package

UDS Core is already running on this cluster. Your job in this lab is to understand how a real UDS package is structured, then deploy it.

The [UDS Reference Package](https://github.com/uds-packages/reference-package) is the canonical example maintained by Defense Unicorns. It's the standard ISVs follow when building UDS-compatible packages.

The repo has been pre-cloned for you. Move into it:

```
cd /root/reference-package
```

In your own environment you'd clone it with:
```
git clone --depth 1 https://github.com/uds-packages/reference-package
```

## What's in the repo

```
ls -1
```

Key directories:

| Path | Purpose |
|------|---------|
| `zarf.yaml` | The Zarf package definition — what gets built and shipped |
| `bundle/` | UDS bundle definitions for deployment |
| `chart/` | Helm chart for the application |
| `values/` | Flavor-specific Helm value overrides |
| `tasks.yaml` | UDS task runner definitions (`uds run dev`, etc.) |

## Read zarf.yaml

```
cat zarf.yaml
```

Notice what is **not** in `zarf.yaml`:

- No Postgres operator
- No cert-manager
- No cluster setup
- No Keycloak deployment

`zarf.yaml` contains exactly one thing: the reference package application. It references a pre-built container image from `ghcr.io` and a local Helm chart. That's the entire scope of a Zarf package — **your app, nothing else**.

## Check the image reference

```
grep -A3 "images:" zarf.yaml
```

The container image is pulled from `ghcr.io` at build time and bundled into the Zarf archive. When deployed in an air-gapped environment, no external registry is needed — everything is in the `.tar.zst` package file.

## Verify

```
ls chart/ values/ bundle/
```
