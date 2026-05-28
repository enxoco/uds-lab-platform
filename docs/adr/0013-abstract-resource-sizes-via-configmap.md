# ADR-0013: Abstract Resource Sizes Mapped via Operator ConfigMap

**Status:** Accepted

## Context

Scenarios have different resource requirements. The previous platform used Hetzner-specific server type strings in `scenario.yaml` (e.g., `serverType: ccx23`) to override the default VM size per scenario (ADR-0005). This coupling to a specific cloud provider's instance naming is incompatible with a multi-provider architecture.

Different providers express resource sizing differently:
- KubeVirt: `resources.requests.cpu`, `resources.requests.memory` on the VMI spec
- Azure VMSS: VM SKU strings (e.g., `Standard_D4s_v3`)

## Decision

`scenario.yaml` uses abstract T-shirt size labels (`size: small | medium | large`) instead of provider-specific instance types. The `serverType` field is removed.

The operator ConfigMap (injected via UDS Package variables at deploy time) maps each abstract size to provider-specific values:

```yaml
# Example operator ConfigMap data
provider: kubevirt
sizes:
  small:
    cpu: "2"
    memory: "4Gi"
  medium:
    cpu: "4"
    memory: "8Gi"
  large:
    cpu: "8"
    memory: "16Gi"
```

For VMSS, the same structure maps sizes to SKU strings. The operator reads this ConfigMap at startup and applies the appropriate resource spec when creating VMIs or VMSS instances.

Default sizes are defined in the operator if no ConfigMap override is provided, so the platform works out-of-the-box without mandatory operator configuration.

## Consequences

- Scenario definitions are provider-agnostic — the same `scenario.yaml` works on KubeVirt, VMSS, or future providers.
- Operators can tune resource allocation for their specific cluster capacity without modifying scenario files.
- Adding a new size tier (e.g., `xlarge`) requires updating both the ConfigMap and any scenarios that need it.
- The previous `VM_SERVER_TYPE` environment variable and `serverType` scenario field are removed.
