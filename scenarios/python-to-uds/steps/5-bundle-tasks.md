# Step 5 – Bundle and tasks

The **bundle** is the deployment unit that UDS understands. Even with a single package, the bundle is the correct layer to deploy from — it's what `uds deploy` processes, and it's where operators can inject configuration overrides at deploy time without touching the package itself.

The **tasks file** defines the development workflow so that `uds run dev` does the right thing in one command. It also pulls in the `uds-common` task library, which is the standard way UDS packages share operational tooling.

## Bundle

```
cat > /root/myapp/bundle/uds-bundle.yaml << 'EOF'
kind: UDSBundle
metadata:
  name: myapp-bundle
  description: Bundle deploying My Python Flask App
  version: dev

packages:
  - name: myapp
    path: ../
    ref: dev
EOF
```

`path: ../` is relative to `bundle/uds-bundle.yaml`. It points to the parent directory (`/root/myapp`), where `uds create` will look for `zarf-package-myapp-amd64-dev.tar.zst` after the package is built.

If your app had a Postgres dependency, you'd list `postgres-operator` as a separate package here — pulled from a registry, pinned to a specific version, with resource configuration exposed via overrides:

```yaml
# Example: adding a dependency at the bundle layer
packages:
  - name: postgres-operator
    repository: ghcr.io/uds-packages/postgres-operator
    ref: 1.15.1-uds.4-upstream
    overrides:
      postgres-operator:
        uds-postgres-config:
          values:
            - path: postgresql.numberOfInstances
              value: 1
  - name: myapp
    path: ../
    ref: dev
```

## Tasks

```
cat > /root/myapp/tasks.yaml << 'EOF'
# yaml-language-server: $schema=https://raw.githubusercontent.com/defenseunicorns/uds-cli/refs/heads/main/tasks.schema.json
includes:
  - setup: https://raw.githubusercontent.com/defenseunicorns/uds-common/v1.25.0/tasks/setup.yaml

tasks:
  - name: dev
    description: Build package, create bundle, and deploy on existing cluster
    actions:
      - cmd: uds zarf package create . --confirm --skip-sbom --no-progress
      - cmd: uds create bundle/ --confirm --no-progress
      - cmd: uds deploy bundle/uds-bundle-myapp-bundle-amd64-dev.tar.zst --confirm --no-progress
EOF
```

## Why includes and uds-common?

The `includes` block pulls in external task files and makes them available under a namespace. Here, everything in `uds-common/tasks/setup.yaml` becomes callable as `setup:<task-name>`.

`uds-common` is Defense Unicorns' shared task library. Every production UDS package includes it. It exists because every UDS package needs to solve the same operational problems: bootstrapping Keycloak users for testing, creating bundles, running linters, publishing to registries. Rather than each package reinventing this, `uds-common` provides a standardized implementation that the whole ecosystem shares.

The typical production `tasks.yaml` includes several namespaces:

```yaml
includes:
  - create:  https://raw.githubusercontent.com/defenseunicorns/uds-common/v1.25.0/tasks/create.yaml
  - deploy:  https://raw.githubusercontent.com/defenseunicorns/uds-common/v1.25.0/tasks/deploy.yaml
  - setup:   https://raw.githubusercontent.com/defenseunicorns/uds-common/v1.25.0/tasks/setup.yaml
  - lint:    https://raw.githubusercontent.com/defenseunicorns/uds-common/v1.25.0/tasks/lint.yaml
  - publish: https://raw.githubusercontent.com/defenseunicorns/uds-common/v1.25.0/tasks/publish.yaml
```

For this lab you only need `setup`, which gives you `setup:keycloak-user` — the task that registers test users in Keycloak so you can log in through the browser.

Pinning to a specific version (`v1.25.0`) ensures your tasks behave identically across every developer's machine and in CI. Floating `main` is fine for prototypes; pin for anything that ships.

The three dev commands map to three phases:

| Command | Phase | What it produces |
|---------|-------|-----------------|
| `zarf package create` | Build | `zarf-package-myapp-amd64-dev.tar.zst` |
| `uds create bundle/` | Bundle | `uds-bundle-myapp-bundle-amd64-dev.tar.zst` |
| `uds deploy bundle/...` | Deploy | App running in the `myapp` namespace |

## Verify

```
grep -q "UDSBundle" /root/myapp/bundle/uds-bundle.yaml && \
grep -q "uds-common" /root/myapp/tasks.yaml && \
grep -q "name: dev" /root/myapp/tasks.yaml && \
echo "Bundle and tasks ready"
```
