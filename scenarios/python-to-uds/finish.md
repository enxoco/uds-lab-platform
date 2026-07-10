# Lab Complete

You've authored a UDS package end-to-end and deployed it to a live cluster. The full anatomy:

| File | Purpose |
|------|---------|
| `chart/Chart.yaml` | Helm metadata |
| `chart/templates/deployment.yaml` | Kubernetes workload |
| `chart/templates/service.yaml` | Internal Kubernetes service |
| `chart/templates/uds-package.yaml` | Pepr-managed network policy and mesh config |
| `zarf.yaml` | Zarf package: chart + image reference |
| `bundle/uds-bundle.yaml` | Deployment unit composing packages together |
| `tasks.yaml` | Development workflow: build → bundle → deploy |

From here, the natural next steps are:
- **Add SSO**: add an `sso` section to your Package CR and have Pepr register a Keycloak client
- **Add a database**: wire in `postgres-operator` at the bundle layer, declare egress in the Package CR
- **Add monitoring**: add a `monitor` section to emit a ServiceMonitor for Prometheus

See the **UDS Reference Package** lab for all three of these patterns working together in a production-grade example.
