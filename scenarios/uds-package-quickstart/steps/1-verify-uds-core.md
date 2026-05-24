# Step 1 – Verify UDS Core

UDS Core should now be running. Let's confirm it.

## Check cluster health

```
kubectl get pods -n uds-dev-stack
```

You should see pods for **Pepr**, **Istiod**, **Keycloak**, and supporting services all in `Running` state.

## Key namespaces UDS Core created

```
kubectl get namespaces | grep -E 'uds|istio|keycloak'
```

| Namespace | What lives there |
|---|---|
| `uds-dev-stack` | Pepr operator, monitoring stack |
| `istio-system` | Istiod control plane |
| `keycloak` | Keycloak SSO |

## How UDS Core works

Every namespace that runs a UDS Package gets:
- A **sidecar proxy** (Envoy via Istio) injected automatically
- **NetworkPolicies** generated from the `Package` CR you write
- **AuthorizationPolicies** enforcing mesh-level access control

The **Pepr** operator watches for `kind: Package` resources and reconciles all of this. You write one YAML; Pepr does the rest.

## What `*.uds.dev` resolves to

`uds.dev` is a public wildcard DNS entry that resolves to `127.0.0.1`. So `hello-uds.uds.dev` will hit your local cluster without any `/etc/hosts` editing.

> **Note:** In this environment port forwarding handles the last hop. In a real local setup you'd visit the URL in a browser directly.

## Verify

When all `uds-core` pods are `Running`, click **Check** to continue.
