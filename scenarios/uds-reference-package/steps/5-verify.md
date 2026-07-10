# Step 5 – Verify integrations

The reference package exercises every major UDS Core integration. Verify each one.

## Generated mesh resources

Pepr turned the single `Package` CR into real Kubernetes and Istio objects:

```
uds zarf tools kubectl get virtualservice,networkpolicy,authorizationpolicy -n reference-package
```

You should see:
- A `VirtualService` routing `reference-package.uds.dev` → your pod
- `NetworkPolicy` resources matching each `allow` rule from the CR
- `AuthorizationPolicy` resources enforcing mesh-level access

Compare the VirtualService count now versus what you noted in step 3 — it should have increased by at least one.

## Ambient mesh: no sidecar

```
uds zarf tools kubectl get pods -n reference-package
```

Note **1/1** in the READY column — your app container only, no Istio sidecar injected. With ambient mesh, ztunnel handles mTLS and traffic capture at the node level. The mesh is fully active even though you can't see it in the pod spec.

## Keycloak SSO

Before you can log in, bootstrap the test user. The reference package uses `uds-common` tasks for this:

```
cd /root/reference-package && uds run setup:keycloak-user --with group="/UDS Core/Admin"
```

This creates two users in Keycloak:
- **admin** user (member of `/UDS Core/Admin`)
- **doug** — the standard UDS test user, password: `unicorn123!@#UN`

Once the command finishes, click the **reference-package** chip in the browser panel above and log in as `doug` with that password. You'll be redirected through Keycloak and land on the reference package UI — the full OIDC flow driven by the client Pepr registered.

Check the generated secret:

```
uds zarf tools kubectl get secret reference-package-sso -n reference-package \
  -o jsonpath='{.data.KEYCLOAK_CLIENT_ID}' | base64 -d && echo
```

Pepr created this secret and populated it from the `secretTemplate` in the Package CR. The app reads its Keycloak credentials from here — no hardcoded values, no manual Keycloak configuration.

## Postgres

```
uds zarf tools kubectl get pods -n postgres
uds zarf tools kubectl get postgresql -n postgres
```

The postgres-operator deployed a managed PostgreSQL instance. The reference package connected to it using credentials injected via bundle overrides — the package itself has no hardcoded database configuration.

## Monitoring

```
uds zarf tools kubectl get servicemonitor -n reference-package
```

Pepr created a `ServiceMonitor` from the `monitor` section in the Package CR. UDS Core's Prometheus instance automatically picks this up and begins scraping `/metrics` on the reference package pod.

## Verify

```
uds zarf tools kubectl get package reference-package -n reference-package \
  -o jsonpath='{.status.phase}'
```

Output: `Ready`
