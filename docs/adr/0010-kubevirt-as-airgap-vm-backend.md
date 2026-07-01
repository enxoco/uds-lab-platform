# ADR-0010: KubeVirt as Airgap VM Backend

**Status:** Accepted

## Context

The platform currently provisions lab VMs via the Hetzner Cloud API. This creates an external dependency that prevents deployment in airgapped environments — a requirement for DoD and other classified/restricted networks. UDS Core clusters deployed in these environments have no outbound internet access.

The platform is planned to be rewritten in a new repository targeting UDS Core as the deployment environment. In that context, the platform itself runs inside a k8s cluster, making cluster-native VM provisioning viable.

KubeVirt runs full virtual machines as `VirtualMachineInstance` (VMI) objects inside k8s. Guest VMs retain full OS, systemd, and network stack — the existing lab VM software (ttyd, tmux, lab-inject.py, noVNC) runs unchanged inside KubeVirt VMIs.

Azure VMSS remains a supported alternative for cloud-connected deployments where KubeVirt is not preferred.

## Decision

KubeVirt is the primary VM backend for the rewritten platform. Hetzner Cloud is dropped entirely. The provider is selected cluster-wide at deploy time (see ADR-0011). In airgap deployments, all VM disk images are pre-bundled as OCI artifacts in the Zarf/UDS package (see ADR-0012).

Because the platform and VMIs share the same cluster, VMIs get cluster-internal DNS and can reach Istio VirtualServices (`*.uds.dev`) directly via CoreDNS — no external DNS or port-forwarding required. Browser Mode scenarios continue to work: the in-VM Chromium hits `*.uds.dev` on loopback-equivalent cluster networking.

## Consequences

- Platform has no external cloud provider dependency — fully airgap capable.
- KubeVirt must be installed in the target cluster (shipped as a UDS Package component).
- VM images must be pre-built as qcow2 and bundled in the Zarf package — no runtime snapshot discovery.
- VMIs consume cluster node resources (CPU, RAM, storage) — cluster must be sized accordingly.
- In-cluster networking eliminates the need for public IPs and SSH key management.
- Azure VMSS provider remains available for cloud deployments via provider selection config.
