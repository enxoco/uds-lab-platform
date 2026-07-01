# Handoff — KubeVirt backend migration (branch `feat/kubevirt-backend`)

Transient working note for a fresh agent/session (e.g. the one starting **on the k3s
VM**). Delete before the PR merges. Authoritative detail lives in the referenced files —
this doc only covers state + how to continue.

## Read these first (committed on this branch)
- **`docs/kubevirt-migration-plan.md`** — full plan, the live **status table**, decisions
  D1–D3, both staff-engineer reviews. Start here.
- **`docs/kubevirt-validation-runbook.md`** — VM + k3s + UDS Core + local KubeVirt-package
  build + bundle-wiring inputs to capture.
- **`docs/adr/0010`–`0013`** — the decisions being implemented.
- Git history on `feat/kubevirt-backend`: commits for Phase A, Phases B+C, and these docs.

## Where things stand
- **Phases A, B, C are committed.** Everything **compiles and unit-tests pass**
  (`go build ./... && go test ./...`). The KubeVirt reconcile path is **unverified on a
  live cluster** — that's the immediate work.
- Still on the **Hetzner path** for the running server (Phase E not started); the operator
  is a parallel new binary. Nothing is wired into the bundle yet (Phase F not started).

## Immediate next steps (in order)
1. **Stand up the env** per the runbook: Azure `Standard_D8s_v3` → verify `/dev/kvm` → k3s →
   UDS Core (core-slim-dev, *not* k3d) → build the **KubeVirt package locally** (+ CDI).
2. **Capture the bundle-wiring inputs** listed in runbook §5 (package name/version/path,
   CDI inclusion, component names, namespaces, Exemption presence, `useEmulation` off).
   Then `bundle/uds-bundle.yaml` can be wired (ordered: KubeVirt → CDI → uds-lab-platform).
3. **Run the Phase 0 spike** (existential): boot the current uds-core image as a VMI via CDI
   and confirm the **nested k3d inside the VMI** starts and serves `*.uds.dev`. Everything
   downstream depends on this working.
4. **Resolve the provider TODOs** (marked in `internal/provider/kubevirt/provider.go`):
   real image tier→OCI refs, DataVolume size/strategy + `storageClassName: local-path`,
   and the NetworkPolicy egress the in-VM workload actually needs.
5. Then proceed Phase D0 → live-test C → Phase E (thin server) → Phase F (packaging).

## Environment / gotchas for the next session
- **Go 1.26 required for local builds.** The original dev machine had no Go, so a toolchain
  was fetched to `~/.local/goroot` *on that machine only* — a fresh VM needs its own Go 1.26
  (`export PATH=.../go/bin:$PATH`). Containerized builds use `golang:1.26-alpine` (Dockerfile).
- **Commit signing**: this branch's earlier commits are **unsigned**; plan is to **re-sign
  the whole branch on the VM** (set up SSH signing there: `gpg.format=ssh`,
  `user.signingkey=<pubkey>`, `commit.gpgsign=true`, email
  `34722899+enxoco@users.noreply.github.com`, then `git rebase --exec 'git commit --amend
  --no-edit -S' <base>`). The docs commit (`4ab0381`) is already signed.
- **A save hook reformats Go files** (gofmt; also injects `_ =` on ignored errors and a
  `/healthz` route). Expect tidy-ups to appear on write; don't fight them.
- **The branch may not be pushed yet** — push before cloning on the VM.
- Deepcopy for the CRD is **hand-written** (`api/v1alpha1/zz_generated.deepcopy.go`); no
  controller-gen. If you add/refactor CRD fields, update it by hand or introduce
  controller-gen.

## Suggested skills
- **`verify`** — once the operator is deployed on the VM, drive a real `POST /api/sessions`
  → LabSession → VMI → Ready flow and observe it, rather than trusting compile-only state.
- **`code-review`** — review the (currently unverified) Phase C operator/provider diff
  before it merges, especially the KubeVirt VMI/DataVolume/NetworkPolicy shapes.
- **`tdd`** — for Phase E's thin-server refactor (LIST-by-clientID quota / TOCTOU has real
  edge cases worth test-first).
- The repo's plan/runbook are the source of truth; no need to re-plan from scratch.
