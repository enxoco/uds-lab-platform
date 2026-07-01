# Plan: Switch the VM backend to KubeVirt (ADRs 0010–0013)

> **Status (living section — update as work lands).** Branch `feat/kubevirt-backend`,
> single large PR off `master`.
>
> | Phase | State |
> |---|---|
> | A — size abstraction (ADR-0013) | ✅ committed (`internal/sizing`, `scenario.Size`, interim Hetzner size map) |
> | B — LabSession CRD + scaffolding | ✅ committed (`api/v1alpha1`, deepcopy, CRD, `uds-lab-vms` ns) |
> | C — operator + KubeVirt provider | ✅ committed, **compiles + unit-tests pass, UNVERIFIED on a live cluster** (`cmd/laboperator`, `internal/provider/kubevirt`, `internal/controller`, `internal/cloudinit`, `internal/operator`) |
> | 0 — nested-virt spike | ⏳ in progress — see `docs/kubevirt-validation-runbook.md` |
> | D0/D1 — qcow2/OCI images + CDI | ⬜ not started |
> | E — thin server refactor | ⬜ not started (still on the Hetzner path) |
> | F — packaging + docs | ⬜ not started |
>
> **Validation-environment decisions (made with the user):**
> - Validate on a **standalone nested-virt Azure VM running k3s directly** (not k3d):
>   k3d's container-nodes don't expose `/dev/kvm`, forcing slow software emulation that
>   can't run our nested-k3d-in-VM-runs-UDS-Core scenario. k3s on the VM gives native KVM.
> - The DU KubeVirt UDS package is entitlement-gated and the user lacks pull access, **but
>   has the source** → KubeVirt package is **built locally** and referenced by `path:` in
>   the bundle. CDI source/inclusion TBD from the local build.
> - On native-KVM k3s, `useEmulation` stays **off**.
> - Bundle wiring (`bundle/uds-bundle.yaml`) is **pending** the locally-built package's
>   name/version/components/namespaces/Exemption details.
>
> **A local Go 1.26 toolchain was fetched to `~/.local/goroot` on the original dev machine**
> because the host lacked Go; a fresh VM session needs Go 1.26 for local `go build`
> (containerized builds via the `golang:1.26-alpine` Dockerfile don't).
>
> See also: `docs/kubevirt-validation-runbook.md` (env setup) and `docs/adr/0010`–`0013`.

---

## Context

The UDS Lab Platform currently provisions lab VMs through the **Hetzner Cloud API**
(`internal/hetzner/client.go`), with all VM lifecycle held in-memory in
`internal/session/manager.go`. This is a hard external dependency that makes airgap
deployment (DoD / classified networks) impossible, and the in-memory model loses all
sessions on server restart.

ADRs 0010–0013 decide the target architecture: KubeVirt as the in-cluster VM backend
(0010), a `LabSession` CRD reconciled by a dedicated operator with a thin server (0011),
OCI-wrapped qcow2 images via CDI DataVolumes (0012), and abstract `size` tiers mapped to
resources via an operator ConfigMap (0013).

**Decision (confirmed with user):** Full refactor (0010–0013) **in this repo**, on a new
branch off `master`. Hetzner removed, not feature-flagged. Azure VMSS is out of scope but
the provider seam must exist.

### CRITICAL topology fact (verified in code — corrects ADR-0010's framing)

Each lab VM runs its **own self-contained nested k3d UDS Core cluster inside the guest**:
`packer/scripts/playground-uds-core.sh` runs `uds deploy k3d-core-slim-dev` at image-build
time and `k3d cluster stop`; on boot `setup.sh` does `k3d cluster start`. `vm/lab-inject.py`
(`:96-99`) runs `kubectl get virtualservices -A` against the **in-VM** kubeconfig
(`/root/.kube/config`). `*.uds.dev` → `127.0.0.1` resolves to the cluster running *inside
the VM*. The host cluster's Istio/CoreDNS is **irrelevant** to how these scenarios work.

Implications the migration MUST honor:
- The nested topology is **preserved**, not replaced with host-cluster DNS. ADR-0010's
  "VMIs reach host Istio `*.uds.dev` directly" rationale is wrong for these scenarios;
  Browser Mode and Services auto-detection already worked without Hetzner and continue to
  target the in-VM cluster.
- Under KubeVirt the stack becomes **node → pod → QEMU VM → k3d containers → UDS Core**.
  This requires **nested virtualization** (`/dev/kvm` on nodes) AND nested containerd
  inside the guest. This is the single highest-risk, currently-unproven assumption.

---

## Phase 0 — De-risking spike (HARD GATE)

Do not trust Phases C+ on a cluster until these pass. See
`docs/kubevirt-validation-runbook.md` for the concrete VM/k3s setup.

1. **Nested k3d in a KubeVirt VMI (existential).** On the k3s-on-VM box: install
   KubeVirt + CDI; convert the *current* uds-core Hetzner snapshot to qcow2 once; import
   via CDI; boot as a VMI; `k3d cluster start`; confirm `kubectl get virtualservices`
   returns and in-VM Chromium reaches `*.uds.dev`. Validates ADR-0010/0012 end-to-end.
2. **CDI clone latency** for a 10–20GB qcow2 on `local-path`. If >~2min, settle the
   DataVolume strategy (D3) now.
3. **etcd object-size check.** Render the uds-core scenario's cloud-init and measure bytes
   (informs D1). Note: D1 already chose operator-side embed, so this is a sanity check.
4. **NetworkPolicy server→Service.** Apply the deny-VMI↔VMI policy and curl a Service from
   a pod in the *server's* namespace.
5. **One-session-per-client concurrency.** Prototype create = LIST CRs by `clientID` +
   create; hammer with two concurrent requests; confirm whether admission/locking is needed.

---

## Decisions promoted out of "open questions" (each blocks Phase C)

- **D1 — Scenario content delivery to the operator.** **Chosen:** operator carries its own
  embedded copy of `vm/` + `scenarios/` (via the root `embed.go`) and renders cloud-init
  operator-side (`internal/cloudinit`). Server writes only `scenarioID`+`size` on the CR.
- **D2 — VMI vs VirtualMachine.** **Chosen: bare VMI.** Ephemeral/TTL'd; a node reboot
  destroys it (acceptable). "Crash recovery" means the *operator* recovers by listing, not
  that VMIs survive node loss.
- **D3 — DataVolume strategy.** Decided by Phase 0 step 2. Default: golden-PVC + CDI clone
  per session; the provider currently issues a per-session CDI registry clone (TODO marked).

---

## Target architecture (what changes)

| Concern | Today (Hetzner) | Target (KubeVirt) |
|---|---|---|
| VM create | `hetzner.CreateServer` → `(VMID, VMIP)` | Server creates `LabSession` CR; operator reconciles → VMI + Service + NetworkPolicy |
| VM identity | `Session.VMID int64`, `Session.VMIP string` | `LabSession` CR; proxy target = `status.serviceDNS` |
| Image selection | `FindLatestSnapshot` by label keyed on `image`/`playground` | `(image tier, size)` → OCI ref in operator config; CDI DataVolume |
| Sizing | `serverType: ccx23` / `VM_SERVER_TYPE` | `size: small\|medium\|large` → operator ConfigMap → `resources.requests` |
| Lifecycle/TTL | in-memory map + `cleanupLoop` | operator reconcile loop; CR `spec.expiresAt` |
| Networking | `http://<publicIP>:<port>` | `http://lab-<id>.uds-lab-vms.svc.cluster.local:<port>` |
| Boot config | server-rendered cloud-init | operator-rendered `cloudInitNoCloud` (same scripts; SSH-key bits dropped) |
| In-VM cluster | nested k3d UDS Core | **unchanged — preserved** |

Reusable, unchanged: `internal/proxy`, `vm/lab-inject.py`, the in-VM software/services,
the web UI, all scenario content, the nested-k3d topology.

---

## Phased work breakdown

### Phase A — ADR-0013 size abstraction ✅
1. `internal/scenario/scenario.go`: `ServerType` → `Size` (validate `small|medium|large`,
   default `medium`). Keep `Image`. Inline re-parse in `manager.go` updated.
2. `scenarios/uds-reference-package/scenario.yaml`: `size: large`.
3. `internal/sizing`: tiers + defaults + `Normalize`/`Valid` (no k8s dep) + `Resolve(overrides)`.
4. Removed `VM_SERVER_TYPE` / `VMConfig.ServerType`; interim size→Hetzner map until Phase E.

### Phase B — LabSession CRD + scaffolding ✅
1. `controller-runtime` v0.20 + `kubevirt.io/api` + CDI api.
2. `api/v1alpha1` LabSession (spec: sessionID/scenarioID/clientID/size/browserEnabled/
   expiresAt; status: phase/serviceDNS/message) + hand-written deepcopy (no controller-gen).
3. `deploy/crd/lab.uds.dev_labsessions.yaml`, `deploy/namespace.yaml` (`uds-lab-vms`).

### Phase C — Lab Operator + KubeVirt provider ✅ (compiles; unverified on cluster)
1. `cmd/laboperator`; `internal/provider` seam; `internal/provider/kubevirt` impl.
2. Reconcile builds: CDI DataVolume (registry clone), VMI (`cloudInitNoCloud`,
   size-resolved `resources.requests`, virtio disks), headless Service (7680/7681/7682
   +6080 when browser), bidirectional NetworkPolicy. Owner refs for GC; idempotent via
   `controllerutil.CreateOrUpdate`.
3. `internal/controller` reconciler: finalizer teardown, TTL expiry, two-phase readiness
   (VMI `Running` + ttyd `:7681` probe → `Ready`).
4. `internal/cloudinit` renders user-data operator-side (D1); `internal/operator` loads the
   size/image ConfigMap.
> **Open TODOs in the provider (marked in code):** image tier→OCI ref map needs real refs
> (Phase D); DataVolume size/strategy + storage class from the Phase 0 spike; confirm the
> NetworkPolicy egress the in-VM workload actually needs.

### Phase D0 — Base image as qcow2/OCI + CDI (prerequisite to live Phase C test)
Rewrite `packer/*.pkr.hcl` + `build-images.sh` onto the **QEMU builder** (qcow2), chained
via file inputs; ORAS-wrap to a cluster-local registry; CDI + storage class confirmed.

### Phase D1 — Heavy images + CI
uds-core qcow2 build + ORAS publish (optional component); CI on a KVM-capable runner + VMI
smoke test.

### Phase E — Thin server refactor
1. Gut `internal/session/manager.go` → thin k8s client (`Create`/`Get`/`Delete` LabSession
   CRs; LIST-by-`clientID` quota with TOCTOU handling).
2. Switch the five `main.go` handlers from `sess.VMIP` to `sess.ServiceDNS`, gate on
   `status.serviceDNS != ""`.
3. Rework `ownsSession`/`clientID` to use CR `spec.clientID`.
4. Delete `internal/hetzner`; remove `HCLOUD_TOKEN`/SSH/location/token prompt; remove
   `chart/templates/hcloud-secret.yaml` and the Hetzner egress in `uds-package.yaml`.
5. RBAC: server CRUD on `LabSession` only; operator on VMI/Service/NetworkPolicy/DataVolume.

### Phase F — Packaging & docs
1. `bundle/uds-bundle.yaml`: ordered multi-package — **locally-built KubeVirt** (+ CDI) →
   `uds-lab-platform`. Operator+server Deployments, CRD, RBAC, size ConfigMap, optional
   image components. Pepr `Exemption` for KubeVirt/CDI namespaces if the package lacks one.
2. Rewrite Hetzner sections of `README.md`; correct the `*.uds.dev` topology description.
3. Dev/CI story for KVM availability.

---

## Verification
- **Phase A**: `go build ./... && go test ./...`; scenarios load and `Size` resolves.
- **Phase 0 / D0**: a hand-imported base-image VMI boots ttyd reachable on its Service.
- **Phases C–E** (k3s-on-VM w/ KubeVirt + CDI): `POST /api/sessions` → `LabSession` CR +
  VMI + Service in `uds-lab-vms`; VMI `Running`; nested `k3d cluster start` works and
  `kubectl get virtualservices` returns inside the guest; status `Ready`; terminal +
  Browser Mode proxy over `status.serviceDNS`; Services auto-detect lists in-VM `*.uds.dev`;
  TTL expiry deletes CR+VMI; operator pod restart reconciles; two concurrent creates for one
  client yield exactly one session.
- **Phase F**: bundle deploys the full stack onto a clean k3s+UDS-Core VM.

---

## Staff-engineer reviews (folded in)
- **Plan review**: corrected the ADR-0010 nested-k3d error; added the Phase 0 gate;
  reordered D0 before C; promoted D1–D3; split `internal/sizing`; re-scoped Packer rewrite
  and the five-handler server refactor; made NetworkPolicy server→Service ingress explicit.
- **Bundle review**: DU KubeVirt package is entitlement-gated at
  `registry.defenseunicorns.com/airgap-store/kubevirt`; CDI likely separate; k3d doesn't
  expose `/dev/kvm` (→ k3s-on-VM decision); CDI on `local-path` is RWO/Filesystem; KubeVirt
  needs a UDS Pepr `Exemption` (privileged hostPath virt-handler); enforce bundle deploy
  ordering and decouple VMI readiness from bundle health checks.
