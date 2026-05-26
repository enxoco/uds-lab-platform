# Step 4 – Write the UDS Package CR

The **UDS Package CR** is what makes a Zarf package a *UDS* package. The Pepr operator watches for `kind: Package` resources and generates:

- **VirtualServices** (Istio) — HTTP routing to your service
- **NetworkPolicies** (Kubernetes) — default-deny with explicit allow rules
- **AuthorizationPolicies** (Istio) — mesh-level enforcement

Without this file, your app deploys but gets no ingress and no network access.

## Write `manifests/uds-package.yaml`

```
cat > /root/hello-uds/manifests/uds-package.yaml << 'EOF'
apiVersion: uds.dev/v1alpha1
kind: Package
metadata:
  name: hello-uds
  namespace: hello-uds
spec:
  network:
    expose:
      - service: hello-uds       # must match your K8s Service name
        selector:
          app: hello-uds         # must match your pod labels
        host: hello-uds          # → hello-uds.uds.dev
        gateway: tenant          # "tenant" = user-facing; "admin" = ops UIs
        port: 443
        targetPort: 80           # container port, not Service port

    allow:
      # Everything is default-deny. Declare every flow your app needs.
      - direction: Egress
        selector:
          app: hello-uds
        remoteGenerated: KubeAPI
        description: "K8s API for in-cluster service discovery"
EOF
```

## Key concepts

**`gateway: tenant`** — Routes traffic through the Istio tenant gateway. Use `admin` for internal dashboards (Grafana, Keycloak admin, etc.).

**`host`** — Combined with the cluster domain to produce the external hostname. `hello-uds` → `hello-uds.uds.dev`.

**`allow` with `remoteGenerated: KubeAPI`** — Even simple apps often need to talk to the K8s API for service discovery. Declare it explicitly or you'll see silent timeouts.

**Other common `allow` patterns:**

```yaml
# Talk to another pod in the same namespace
- direction: Egress
  selector:
    app: hello-uds
  remoteNamespace: hello-uds
  remoteSelector:
    app: postgres
  port: 5432
  description: "Postgres in same namespace"

# Reach an external host
- direction: Egress
  selector:
    app: hello-uds
  remoteGenerated: Anywhere
  port: 443
  description: "External HTTPS (S3, APIs, etc.)"
```

## Verify

```
ls /root/hello-uds/manifests/
```
