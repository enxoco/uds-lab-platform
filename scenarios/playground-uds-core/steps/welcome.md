# UDS Core Playground

`k3d-core-slim-dev` is already deployed. UDS Core, Keycloak, and Istio are running.

## Cluster access

```
uds zarf tools kubectl get pods -A
```

## UDS Core services

| Service | URL |
|---------|-----|
| SSO (Keycloak) | https://sso.uds.dev |
| Grafana | https://grafana.admin.uds.dev |

Open the **Browser** tab to access these URLs from inside the VM — they resolve to `127.0.0.1` which is the k3d cluster.

## Deploy a UDS Package

```
uds deploy oci://ghcr.io/defenseunicorns/packages/uds/podinfo:latest --confirm
```
