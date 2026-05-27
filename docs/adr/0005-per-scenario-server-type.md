# ADR-0005: Per-Scenario Server Type Override

**Status:** Accepted

## Context

All scenarios previously shared a single Hetzner server type configured globally via `VM_SERVER_TYPE` (default: `ccx13`, 2 vCPU / 8 GB). This worked for lightweight scenarios but proved insufficient for the `uds-reference-package` scenario, which runs UDS Core (Istio, Keycloak, Pepr) plus postgres-operator plus the reference package application on the same cluster. At 92% CPU request allocation before postgres deploys, the `ccx13` cannot schedule postgres pods despite low actual CPU utilization — Kubernetes refuses to schedule based on requests, not usage.

Alternatives considered:

- **Increase global default to `ccx23`** — raises cost for all scenarios, including lightweight ones that don't need it.
- **Two-VM k3s cluster (server + agent on separate Hetzner VMs)** — genuinely doubles compute (2× `ccx13` = 4 vCPU / 16 GB, same as `ccx23`) and gives real multi-node scheduling. Requires significant platform changes: session manager must provision and coordinate two VMs, inter-VM communication requires a Hetzner private network, k3s agent must join the server using a shared token after server boot, teardown must clean up both VMs, and failure of the agent VM breaks the cluster mid-lab. Cost is identical to `ccx23` but operational complexity is substantially higher. Worth revisiting if scenarios need to demonstrate multi-node Kubernetes behaviour specifically (e.g., node affinity, pod disruption budgets, node failure scenarios) — for resource headroom alone, a single larger VM is preferable.
- **Strip resource requests from all pods** — undermines the production-representative value of UDS Core; breaks the "this is what real UDS looks like" premise of the platform.

## Decision

Add an optional `serverType` field to `scenario.yaml`. When set, the session manager uses it instead of the global `VM_SERVER_TYPE`. Scenarios that don't set it continue to use the global default.

The `uds-reference-package` scenario sets `serverType: ccx23` (4 vCPU / 16 GB).

## Consequences

- Lightweight scenarios stay on `ccx13`; resource-heavy scenarios opt into larger types explicitly.
- Scenario authors are responsible for right-sizing — a `serverType` declaration is a cost decision and should be justified.
- Operators self-hosting the platform should document the server types their scenarios require so infrastructure costs are predictable.
- The global `VM_SERVER_TYPE` env var remains the fallback and can still be used to override all scenarios at the deployment level (e.g., forcing a larger type for all sessions in a resource-constrained environment).
