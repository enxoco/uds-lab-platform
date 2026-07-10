# Step 3 – The UDS Package CR: network, mesh, and SSO

The `Package` CR is what separates a Helm chart from a *UDS package*. Without it, your app deploys but Pepr doesn't know it exists: no external access, no network policies, no mesh integration, no SSO. Pepr watches for `kind: Package` resources and generates all the Kubernetes and Istio plumbing automatically.

## Write the CR

```
cat > chart/templates/uds-package.yaml << 'EOF'
apiVersion: uds.dev/v1alpha1
kind: Package
metadata:
  name: myapp
  namespace: {{ .Release.Namespace }}
spec:
  sso:
    - name: MyApp SSO
      clientId: uds-core-myapp
      redirectUris:
        - "https://myapp.uds.dev/login"
      enableAuthserviceSelector:
        app: myapp
  network:
    serviceMesh:
      mode: ambient
    expose:
      - service: myapp
        selector:
          app: myapp
        gateway: tenant
        host: myapp
        port: 8080
        uptime:
          checks:
            paths:
              - /health
    allow:
      - direction: Ingress
        remoteGenerated: IntraNamespace
      - direction: Egress
        remoteGenerated: IntraNamespace
EOF
```

## What each section does

### `sso` and `enableAuthserviceSelector`

The `sso` block registers a Keycloak OIDC client for your app. Pepr creates the client automatically — no manual Keycloak configuration required.

`enableAuthserviceSelector` goes one step further: instead of the Flask app implementing OIDC itself, **AuthService** handles authentication transparently at the Istio layer. Every request to `myapp.uds.dev` is intercepted by an Envoy filter before it reaches your pod. If the request doesn't carry a valid Keycloak session, AuthService redirects to Keycloak login. If it does, the request is forwarded to your app.

This means a Flask app with zero auth code gets full SSO protection. The selector `app: myapp` tells AuthService which pods to protect — it matches the pod label you set in the Deployment.

See the full AuthService guide: https://docs.defenseunicorns.com/core/how-to-guides/identity--authorization/protect-apps-with-authservice/

### `serviceMesh.mode: ambient`

UDS Core uses Istio **ambient mesh** — mTLS and observability are handled at the node level by ztunnel, not by a per-pod sidecar proxy. Your pod will show `1/1` READY (not `2/2`) because no Istio container is injected alongside your app. The mesh is fully active; it's just not visible inside the pod.

### `network.expose`

Pepr reads this and creates an Istio `VirtualService` that routes HTTPS traffic from outside the cluster to your Service. The fields:

| Field | Value | Effect |
|-------|-------|--------|
| `service` | `myapp` | The Kubernetes Service to route to |
| `gateway` | `tenant` | Routes via the user-facing Istio ingress gateway |
| `host` | `myapp` | Becomes `myapp.uds.dev` (domain appended from cluster config) |
| `port` | `8080` | The container port, matching `targetPort` in the Service |
| `uptime.checks.paths` | `/health` | Pepr creates an uptime check against `myapp.uds.dev/health` |

Use `gateway: admin` for internal dashboards (Grafana, Prometheus) that shouldn't be reachable from the user-facing ingress.

### `network.allow`

**Everything is default-deny.** UDS Core installs a cluster-wide NetworkPolicy that blocks all pod-to-pod traffic by default. Every flow your app needs must be declared explicitly here.

For a simple app with no external dependencies, two rules cover everything:
- `IntraNamespace Ingress` — allows traffic into your pod from within the same namespace (needed for ztunnel's ambient mesh traffic capture)
- `IntraNamespace Egress` — allows outbound traffic within the namespace

For a real app that connects to Postgres, you'd add a third rule:
```yaml
- direction: Egress
  selector:
    app: myapp
  remoteNamespace: postgres
  remoteSelector:
    app: postgres-operator
  port: 5432
  description: "Postgres connection"
```

Pepr translates each `allow` entry into a `NetworkPolicy` and an `AuthorizationPolicy` — you don't write those objects directly.

## Verify

```
grep -q "kind: Package" /root/myapp/chart/templates/uds-package.yaml && \
grep -q "enableAuthserviceSelector" /root/myapp/chart/templates/uds-package.yaml && \
grep -q "network" /root/myapp/chart/templates/uds-package.yaml && \
echo "Package CR looks good"
```
