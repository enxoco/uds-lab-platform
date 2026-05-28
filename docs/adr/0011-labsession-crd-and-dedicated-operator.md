# ADR-0011: LabSession CRD and Dedicated Operator

**Status:** Accepted

## Context

The current platform manages VM lifecycle in-memory inside the platform server (`session/manager.go`). This has two problems at k8s scale:

1. **No crash recovery** — server restart orphans VMs. On Hetzner, orphaned VMs accumulate cost until TTL cleanup runs. On KubeVirt, they consume cluster CPU/RAM/storage indefinitely.
2. **Tight coupling** — the server conflates API/proxy responsibilities with infrastructure provisioning. Adding providers or changing lifecycle logic requires modifying the server binary.

The operator pattern is the standard k8s approach for managing stateful infrastructure lifecycle via reconciliation loops.

## Decision

Split the platform into two binaries:

**Platform Server** — thin API and proxy layer only:
- Serves the lab UI and REST API
- Authenticates users (OIDC, see ADR-0006)
- Enforces per-user session quotas by listing active `LabSession` objects
- Creates and deletes `LabSession` CRD objects
- Proxies WebSocket/HTTP traffic to VMI Services (ttyd, lab-inject, noVNC)

**Lab Operator** — infrastructure lifecycle manager:
- Watches `LabSession` objects
- Reconciles each `LabSession` to: VMI + headless Service + NetworkPolicy
- Enforces TTL by checking `LabSession.spec.expiresAt` and deleting expired objects
- Manages VMI readiness: two-phase (watch VMI phase → `Running`, then HTTP poll ttyd `:7681`) and updates `LabSession.status`
- Routes to correct provider implementation based on `LAB_PROVIDER` env var

`LabSession` CRD carries all session metadata as spec/annotations: `sessionID`, `scenarioID`, `userID`, `size`, `expiresAt`. VMI annotations mirror this for k8s-native state recovery — operator reconstructs in-memory state by listing VMIs on startup.

All VMIs and their Services are created in a dedicated namespace (`uds-lab-vms`) with NetworkPolicy restricting VMI-to-VMI traffic while permitting cluster service access (CoreDNS, Istio).

## Consequences

- Server is stateless — restarts are safe, no session loss.
- Operator crash recovery is free — VMIs survive pod restarts, operator reconciles on resume.
- Clean RBAC boundary: server needs CRUD on `LabSession` only; operator needs create/delete/watch on VMI, Service, NetworkPolicy in `uds-lab-vms` namespace.
- Adding a new provider (e.g., VMSS) requires only a new operator provider implementation — server is unchanged.
- Two binaries to build, deploy, and version — acceptable tradeoff for separation of concerns.
