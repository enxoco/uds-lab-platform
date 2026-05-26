# Step 5 – Verify integrations

The reference package exercises every major UDS Core integration. Verify each one.

## Generated mesh resources

Pepr turned the single `Package` CR into real Kubernetes and Istio objects:

```
uds zarf tools kubectl get virtualservice,networkpolicy,authorizationpolicy -n reference-package
```

You should see:
- A `VirtualService` routing `reference-package.uds.dev` → your pod
- `NetworkPolicy` resources enforcing the `allow` rules from the CR
- `AuthorizationPolicy` resources enforcing mesh-level access

## Keycloak SSO

Once the Package CR reaches `Ready`, a **reference-package** service chip will appear at the top of this page. Click it to open `https://reference-package.uds.dev` in the VM browser — you should be redirected to the Keycloak login page. Pepr registered the SSO client automatically when it reconciled the Package CR.

## Check the generated Keycloak client

```
uds zarf tools kubectl get secret reference-package-sso -n reference-package -o jsonpath='{.data.clientId}' | base64 -d && echo
```

Pepr created this secret and populated it with the Keycloak client credentials. Your app reads from it — no hardcoded secrets, no manual Keycloak configuration.

## Postgres

```
uds zarf tools kubectl get pods -n postgres
uds zarf tools kubectl get postgresql -n postgres
```

The postgres-operator deployed a managed PostgreSQL cluster. The reference package connected to it using credentials injected via bundle overrides — the package itself has no hardcoded database configuration.

## Pod readiness

```
uds zarf tools kubectl get pods -n reference-package
```

Note the **2/2** in the READY column — your app container plus the Istio sidecar injected automatically by UDS Core.

## Verify

```
uds zarf tools kubectl get package reference-package -n reference-package -o jsonpath='{.status.phase}'
```

Output should be `Ready`.
