# You packaged your first UDS app

Here's what you built and why each piece matters:

| File | Purpose |
|---|---|
| `zarf.yaml` | Build manifest — bundles images + charts into an air-gap-ready archive |
| `values/values.yaml` | Bridges `###ZARF_VAR_FOO###` tokens → Helm values at deploy time |
| `manifests/uds-package.yaml` | Tells Pepr to wire up Istio routing, NetworkPolicies, and AuthorizationPolicies |

## What Pepr did for you

One `kind: Package` CR with a 10-line `expose` block generated:
- A VirtualService routing external HTTPS → your pod
- NetworkPolicies enforcing default-deny (only declared traffic flows work)
- AuthorizationPolicies enforcing mesh-level mTLS rules

## What's next

| Goal | Where to go |
|---|---|
| Add Keycloak SSO to your app | Keycloak SSO pattern docs |
| Reach an external service (S3, Azure, etc.) | Add `remoteGenerated: Anywhere` allow rule |
| Manage secrets (DB passwords, API keys) | Secrets management pattern docs |
| Bundle multiple packages together | UDS Bundle (`uds-bundle.yaml`) |

## Key things to remember

- **Never deploy into `namespace: default`** — Zarf's image-rewriting webhook is skipped there
- **Token case matters** — `###ZARF_VAR_FOO###` must be uppercase
- **Everything is default-deny** — declare every egress your app needs in `allow`
- **`kubectl get package -n <ns>`** is your first debug command when things don't work
