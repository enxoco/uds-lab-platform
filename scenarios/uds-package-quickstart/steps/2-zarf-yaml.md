# Step 2 – Write `zarf.yaml` and the values bridge

Change into the package directory:

```
cd /root/hello-uds
```

## 2a — `zarf.yaml`

This is the **build manifest**. It tells Zarf which images to bundle and which Helm chart to deploy.

```
cat > zarf.yaml << 'EOF'
kind: ZarfPackageConfig
metadata:
  name: hello-uds
  version: "0.1.0"
  description: "Hello UDS — sample nginx package"

variables:
  - name: DOMAIN
    default: "uds.dev"
  - name: APP_REPLICAS
    default: "1"

components:
  - name: hello-uds
    required: true
    charts:
      - name: hello-uds
        version: 0.1.0
        namespace: hello-uds
        localPath: chart/
        valuesFiles:
          - values/values.yaml
    manifests:
      - name: uds-package
        namespace: hello-uds
        files:
          - manifests/uds-package.yaml
    images:
      - nginx:1.27
EOF
```

> **Why not `namespace: default`?**
> The `default` namespace has a `zarf.dev/agent=ignore` label. Zarf's image-rewriting webhook won't run there, so pods can't pull from the internal registry. Always use a dedicated namespace.

## 2b — `values/values.yaml`

This file bridges **Zarf variables** → **Helm values**. Zarf does string substitution on `###ZARF_VAR_FOO###` tokens before handing the file to Helm.

```
cat > values/values.yaml << 'EOF'
config:
  domain: "###ZARF_VAR_DOMAIN###"

# No quotes on numerics — "1" renders as the string "1" in Helm
replicaCount: ###ZARF_VAR_APP_REPLICAS###
EOF
```

### Token rules (failures are silent)

| Rule | Example |
|---|---|
| Always uppercase | `###ZARF_VAR_DOMAIN###` ✓ — `###ZARF_VAR_domain###` ✗ |
| No quotes on ints/bools | `replicaCount: ###ZARF_VAR_APP_REPLICAS###` ✓ |
| Must be declared in `zarf.yaml` | Undeclared tokens render literally |

## Verify

```
ls values/ manifests/ && cat zarf.yaml
```
