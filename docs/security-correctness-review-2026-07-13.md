# Security and Correctness Review

Date: 2026-07-13
Branch: `feat/reduce-image-sizes`
Compared with: `origin/master`
Status: remediation in progress

## Verdict

The branch was not safe to merge at review time. The new VM-image packaging and
development workflows contained release-blocking correctness failures, and the
base-image build introduced supply-chain risks that need a deliberate policy.

## Findings

### Critical: unresolved Zarf version template in CDI manifests

`packages/vm-images/manifests/golden-pvcs.yaml` uses
`###ZARF_PKG_TMPL_VERSION###` inside a deploy-time Kubernetes manifest. Zarf
package templates apply to `zarf.yaml` during package creation; deploy-time
manifests use `###ZARF_VAR_*###` values. `zarf dev inspect manifests
packages/vm-images` reproduced the unresolved marker in both registry URLs.

References:

- `packages/vm-images/manifests/golden-pvcs.yaml:17`
- `packages/vm-images/manifests/golden-pvcs.yaml:37`
- [Zarf package templates](https://docs.zarf.dev/ref/create/)
- [Zarf deployment values](https://docs.zarf.dev/ref/values/)

### High: image archive tags diverge after a version bump

The build task tags the generated images from the package version, while
`packages/vm-images/zarf.yaml` hardcodes both archive image references to
`0.1.0`. The version-bump workflow updates package metadata but not these image
references. Zarf requires each image listed in an image archive to exist in the
archive, so later package builds fail or package the wrong image references.

References:

- `packages/vm-images/zarf.yaml:8,41-44`
- `tasks.yaml:269-282`
- `.github/workflows/bump-version.yaml:91-92`
- [Zarf image archives](https://docs.zarf.dev/v0-74/ref/components/)

### High: default development flow replaces packaged images with HTTP imports

`cluster-up` deploys the registry-backed VM-image package and then, by default,
`populate-golden-pvcs` deletes those DataVolumes and imports local qcow2 files
through a temporary host HTTP server. This makes the packaged, registry-backed
path ineffective for the normal full development flow and breaks the expected
air-gapped behavior whenever local qcow2 files are present.

References:

- `tasks.yaml:527,535-537`
- `scripts/create-golden-pvc.sh:143-159`

### Resolved: skip-image and redeploy paths required image artifacts

The image package task silently skips missing qcow2 inputs, then always runs
`zarf package create`. With no `base.tar` or `uds-core.tar`, package creation
fails. This breaks the documented `SKIP_IMAGES=1` and existing-cluster
redeploy workflows when local qcow2 artifacts are not present.

The original workflow required local archives even when the bundle could have
used a published package. The bundle now references the registry-published
`registry.uds-mil.us/enxo/lab-vm-images` package, so normal cluster setup and
redeploy no longer depend on local VM archives. The explicit
`build-vm-images-package` task still validates local archives because it is the
manual package-build and publish path.

Original reproduction:

```text
failed to create package: failed to extract tar: unable to decompress:
opening "packages/vm-images/base.tar": no such file or directory
```

References:

- `tasks.yaml:272-288`
- `tasks.yaml:527`
- `tasks.yaml:726-737`
- `README.md:66-75`

### High: unpinned executable downloads in the base image

The base image executes mutable upstream content as root:

- mutable `yq` release lookup and binary download
- mutable UDS CLI release lookup and binary download
- `k3d` installer piped directly to Bash
- Google Chrome and Docker repository keys without fingerprint verification
- ttyd binary download without a checksum
- CI installs `golangci-lint` from the mutable `HEAD` installer

A compromised upstream, mutable release, or transport/account failure can alter
every generated VM image. Downloads should be pinned and verified with checksums,
signatures, or trusted package metadata. Remote shell installers should be
eliminated or pinned and verified.

References:

- `packer/scripts/base.sh:24-35`
- `packer/scripts/base.sh:231-236`
- `packer/scripts/base.sh:243-254`
- `packer/scripts/base.sh:261-271`

### Medium: deployment action has weak prerequisite and failure handling

The VM-image package's `onDeploy` action requires host `kubectl` and `jq`, but
does not declare or package them. It also succeeds without copying a pull secret
if none of the searched namespaces contains `private-registry`; the CDI
DataVolumes then fail later and less clearly. Zarf actions execute in the
context of the Zarf binary and require their binaries to exist on that machine.

References:

- `packages/vm-images/zarf.yaml:24-37`
- [Zarf actions](https://docs.zarf.dev/ref/actions/)

### Medium: CI permissions exceed the currently enabled behavior

The normal CI job grants `contents: write`, `packages: write`, and `id-token:
write`, even though image build, publish, and release steps are disabled. This
increases the impact of compromised pull-request code or a compromised action.
The job should use read-only permissions unless a narrowly scoped release job
needs elevated permissions.

Reference: `.github/workflows/build.yaml:25-28`.

### Informational: release documentation contradicts workflow state

The README says tag pushes automatically build and publish releases, while the
release steps are currently disabled with `if: false` and are being run
manually.

References:

- `README.md:270-282`
- `.github/workflows/build.yaml:62-99`

### Supply-chain policy: use Cosign when the publisher supplies signatures

The standalone base-image tools currently do not expose a documented Cosign
blob signature and certificate/bundle flow in their official release material:
`yq`, UDS CLI, k3d, and ttyd publish release binaries and/or checksums instead.
Those downloads should therefore use immutable release references plus verified
checksums until the publishers provide Cosign artifacts. UDS package
signatures are a separate mechanism: UDS documents keyless verification for
signed Zarf packages, and should remain the verification path for signed UDS
packages rather than being conflated with binary verification.

When a dependency publishes Cosign signatures, the image build must use
`cosign verify-blob` with the publisher's documented identity/issuer or public
key before installing the binary. A bare `cosign verify-blob` without an
identity or key constraint is not an acceptable control.

## Verification

Passed:

- `GOCACHE=/tmp/uds-lab-go-cache go test ./...`
- `GOCACHE=/tmp/uds-lab-go-cache go vet ./...`
- `shellcheck` and `bash -n` for changed shell scripts
- YAML parsing for workflows, tasks, package manifests, and chart values
- `zarf dev lint packages/vm-images`
- `git diff --check`
- `gitleaks dir --no-banner .`

Not completed:

- Packer validation could not load the QEMU plugin in the review environment.
- OpenGrep could not download its remote rules because network/DNS access was
  unavailable.
- Full cluster deployment was not run.

The review included two independent read-only subreviews plus an adversarial
failure-mode pass.

## Remediation Started

- Fixed package-version propagation by using a package-create template for
  image archive tags and a deploy-time Zarf constant in the CDI manifests.
- Passed the VM-image package version explicitly to `zarf package create`.
- Changed the normal dev flow to use the registry-published VM-image package;
  local HTTP qcow2 import is now an explicit fallback.
- Made missing VM image archives fail immediately for the explicit local image
  package task and retained archive reuse for that task.
- Made missing `private-registry` secrets fail the package deployment action.
- Reduced normal CI package/content permissions to read-only where release
  writes are currently disabled.
- Pinned the CI `golangci-lint` installer to release `v2.11.4`.
- Replaced floating base-image tool downloads with pinned releases and
  checksum verification for `yq`, k3d, ttyd, and UDS CLI.
- Pinned UDS CLI to `v0.33.0`, which understands the keyless verification
  metadata on the UDS Core `k3d-core-slim-dev` bundle.
- Corrected release documentation to describe the current manual process.

Still outstanding: configure the exact Fulcio certificate identity constraint
for the registry-published VM-image package. The pinned standalone binary
downloads do not currently have publisher-provided Cosign blob signatures to
verify.
