# Step 4 – Write zarf.yaml

`zarf.yaml` defines the **Zarf package** — the unit of build, ship, and deploy. A Zarf package for a single app is intentionally minimal: it lists the Helm chart to install and the container image to bundle. Nothing more.

```
cat > /root/myapp/zarf.yaml << 'EOF'
kind: ZarfPackageConfig
metadata:
  name: myapp
  description: "My Python Flask app packaged for UDS"
  version: dev

components:
  - name: myapp
    required: true
    charts:
      - name: myapp
        localPath: chart/
        namespace: myapp
        version: 0.1.0
    images:
      - myapp:dev
EOF
```

## What Zarf does with this

When you run `zarf package create`:

1. Zarf reads the `images` list and looks up `myapp:dev` in the local Docker daemon
2. All image layers are bundled into a `.tar.zst` archive alongside the Helm chart files
3. The result — `zarf-package-myapp-amd64-dev.tar.zst` — is fully self-contained

This is the air-gap story. The package can be copied to a disconnected environment and deployed with no registry access. The image travels with the package.

## Two things that are NOT in zarf.yaml

**No cluster setup.** No k3d, no CNI, no cert-manager. The cluster is someone else's responsibility — in this lab it's already running.

**No infrastructure operators.** If your app needed Postgres, you wouldn't add `postgres-operator` here. Shared operators go in the bundle layer (next step), where they can be versioned independently and reused by multiple packages.

A Zarf package contains exactly one application and its direct Helm chart dependencies. Everything else goes at the bundle layer.

## Verify

```
grep -q "ZarfPackageConfig" /root/myapp/zarf.yaml && \
grep -q "myapp:dev" /root/myapp/zarf.yaml && \
echo "zarf.yaml looks good"
```
