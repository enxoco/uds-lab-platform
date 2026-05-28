# ADR-0012: OCI-Wrapped qcow2 Image Distribution via Zarf

**Status:** Accepted

## Context

The current platform uses Hetzner snapshots for VM images, discovered at runtime via label selectors. Snapshots are Hetzner-proprietary and require outbound API access — incompatible with airgap deployments.

KubeVirt uses `DataVolume` objects (via CDI — Containerized Data Importer) to provision VM disk images. DataVolumes can pull from HTTP sources or OCI registries. In airgap, only a cluster-local registry is available.

VM images are built in layers (base → tools → uds-core) via Packer. The uds-core image is large (~10–20GB qcow2) because it pre-installs a k3d cluster with all UDS Core container images — this bake-time pre-loading is required for fast lab session startup in airgap (no runtime pulls possible).

## Decision

VM images are built as qcow2 files using the Packer QEMU builder (replacing Hetzner-specific Packer builders). CI publishes qcow2 files as OCI artifacts to a registry via ORAS. The Zarf/UDS bundle includes the OCI-wrapped qcow2 images as package components.

At deploy time, Zarf loads images into the cluster-local registry. KubeVirt `DataVolume` objects reference the cluster-local registry URL — CDI clones the qcow2 into a PVC when a new `LabSession` is created.

The uds-core playground image is an **optional** UDS Package component. Operators deploying to resource-constrained airgap clusters can omit it. Base and tools images are mandatory.

Image label selectors (used for runtime discovery on Hetzner) are replaced by static image references in operator config — each abstract size tier maps to a specific OCI image digest in the cluster registry.

## Consequences

- Platform has zero runtime dependency on external image registries.
- Zarf bundle size increases significantly when uds-core image component is included (~10–20GB).
- qcow2 builds require QEMU/KVM access in CI — self-hosted runners with nested virt support.
- DataVolume clone adds latency to session creation (30s–3min depending on storage backend) — mitigated by two-phase readiness polling (ADR-0011).
- Image updates require rebuilding and rebundling the Zarf package — no hot-patching of running clusters.
- ORAS tooling is added to the CI pipeline for OCI artifact publishing.
