# Step 2 – Bundle layer: dependencies at the right layer

This is the most common mistake ISVs make when first building UDS packages.

## The wrong way

Imagine a developer who needs Postgres. They add it directly to `zarf.yaml`:

```yaml
# DON'T DO THIS
components:
  - name: postgres-operator
    charts:
      - name: postgres-operator
        url: https://opensource.zalando.com/postgres-operator
        version: "1.15.1"
        namespace: postgres
  - name: my-app
    charts:
      - name: my-app
        localPath: chart/
        namespace: my-app
```

This breaks UDS composability in three ways:

1. **Version conflicts** — if two packages bundle different postgres-operator versions, the second deploy overwrites the first. No negotiation, no error.
2. **No bundle overrides** — bundle operators can't configure the operator (connection limits, storage class, instance count) at deploy time if it's buried inside a package.
3. **Operator duplication** — every app that needs Postgres brings its own operator. In a production cluster with 10 UDS packages, you get 10 postgres-operator deployments fighting each other.

## The right way: `bundle/uds-bundle.yaml`

```
cat bundle/uds-bundle.yaml
```

The bundle is the deployment unit that composes packages together. Infrastructure dependencies like `postgres-operator` live here — pulled from a registry at a pinned version, with full override capability exposed to the operator deploying the bundle.

The reference package itself (`path: ../`) is also listed as a package in the bundle. The bundle wires them together at deploy time via `overrides` — telling the reference package where to find its Postgres credentials, which SSO secret to use, and whether to enable monitoring.

## `uds run dev` deploys this bundle

```
cat tasks.yaml | grep -A5 "dev:"
```

`uds run dev` reads `tasks.yaml` and deploys `bundle/uds-bundle.yaml` against the running cluster. This is the correct workflow for iterating on a package with its full dependency chain during development. You get postgres-operator + your app in one command, wired together exactly as they would be in production.

The `default` task (not used here) also provisions a fresh k3d cluster — useful in CI but unnecessary when UDS Core is already running.

## Verify

```
grep -r "postgres-operator" zarf.yaml; echo "exit: $?"
```

Postgres is not in `zarf.yaml`. Exit code 1 (not found) confirms the correct structure.
