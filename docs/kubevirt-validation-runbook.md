# KubeVirt Validation Runbook (Phase 0 — k3s on a nested-virt VM)

This is the environment setup for validating the KubeVirt backend (ADRs 0010–0013)
on a **single standalone VM running k3s directly** — chosen over k3d so KubeVirt
gets **native `/dev/kvm`** (k3d's container-nodes do not expose it; that path only
supports slow software emulation, which is non-viable for our nested
k3d-in-VM-runs-UDS-Core scenario).

Target shape: `node (k3s on VM) → KubeVirt VMI → nested k3d → UDS Core`.

---

## 1. Provision the VM

Azure, **nested-virtualization-capable** size (Dsv3/Esv3 family). Suggested:
`Standard_D8s_v3` (8 vCPU / 32 GiB) for one concurrent `large` lab; `D16s_v3` for
headroom.

```sh
az group create -n uds-lab-spike -l eastus
az vm create \
  -g uds-lab-spike -n uds-lab-spike \
  --image Ubuntu2404 \
  --size Standard_D8s_v3 \
  --admin-username azureuser \
  --generate-ssh-keys \
  --os-disk-size-gb 128
```

Disk: the uds-core qcow2 is 10–20 GB and CDI clones it onto `local-path`, so give
the OS disk real room (≥128 GB).

## 2. Verify nested virtualization

```sh
egrep -c '(vmx|svm)' /proc/cpuinfo   # must be > 0
ls -l /dev/kvm                        # must exist
sudo apt-get update && sudo apt-get install -y cpu-checker libvirt-clients
sudo kvm-ok                           # "KVM acceleration can be used"
sudo virt-host-validate qemu          # PASS on the KVM checks
```

If `/dev/kvm` is missing, the Azure size does not expose nested virt — stop and
pick a Dsv3/Esv3 size.

## 3. Install k3s

```sh
curl -sfL https://get.k3s.io | sh -
sudo k3s kubectl get nodes
mkdir -p ~/.kube && sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config \
  && sudo chown $(id -u):$(id -g) ~/.kube/config
export KUBECONFIG=~/.kube/config
```

k3s ships `local-path` as the default StorageClass — that's what CDI DataVolumes
will use (RWO, Filesystem mode).

## 4. Deploy UDS Core onto the existing k3s

Do **not** use `k3d-core-slim-dev` here — that flavor spins up its own k3d. For a
pre-existing cluster, deploy the core (slim-dev) package against this k3s.

```sh
# CONFIRM the exact package ref/version you intend to use:
uds deploy oci://ghcr.io/defenseunicorns/packages/uds/core-slim-dev:<VERSION> --confirm
```

> TODO(confirm): pin the core-slim-dev package ref/version DU wants for this spike.
> Verify it deploys cleanly onto bare k3s (Istio, Pepr, Keycloak, etc. come up).

## 5. Build the KubeVirt package locally — capture this for bundle wiring

The DU KubeVirt UDS package is entitlement-gated and not pullable here, but its
**source is available**, so build the package on (or for) the VM and reference it
by local `path:` in the bundle rather than a remote `repository:`.

```sh
# clone the KubeVirt package source (fill in the repo you have)
git clone <KUBEVIRT_PACKAGE_SOURCE_REPO> kubevirt-package
cd kubevirt-package

# build the zarf/uds package locally (commands depend on the source layout —
# typically one of these, check its tasks.yaml / README):
uds run build            # if it defines a build task
# or
zarf package create . --confirm --output ./build
# or
uds create . --confirm

# inspect what you just built so the bundle can reference it accurately
zarf package inspect ./build/zarf-package-*.tar.zst        # name, version, components, images
```

### Capture these answers (paste back so the bundle gets wired correctly):
1. **Package name + version** and the **built artifact path** (tarball) — the bundle
   references it via `path:`/`ref:`, not a remote registry.
2. **CDI**: built into the same package, a **separate** package to build too (give its
   path), or absent (→ we vendor upstream CDI as our own Zarf component).
3. **Component / chart names** inside each package (needed for `overrides` paths).
4. **`useEmulation`**: confirm it is **not** force-enabled. On native-KVM k3s we want
   real virtualization (`spec.configuration.developerConfiguration.useEmulation: false`).
   If the package hardcodes `true`, note it so we override it off.
5. **UDS Exemption**: does the package ship a Pepr policy `Exemption` CR for the KubeVirt
   (and CDI) namespaces? If not, we author one — `virt-handler` is a privileged hostPath
   DaemonSet and UDS Core's policies will block it otherwise.
6. **Namespaces** the package installs into (for NetworkPolicy/Exemption scoping).
7. **Storage / DataVolume assumptions**, if any (default SC, access modes). On k3s this
   should be `local-path` / RWO / Filesystem.

> Once these are captured, the bundle (Section 6) is wired with real local refs and the
> two-command deploy flow works.

---

## 6. Bundle workflow this enables (wired after inspect)

Once the refs are confirmed, `bundle/uds-bundle.yaml` becomes an ordered
multi-package bundle:

```
KubeVirt (operator + CR + Exemption) → CDI (operator + CR) → uds-lab-platform
```

Deploy flow on the VM:

```sh
uds deploy oci://.../core-slim-dev:<VERSION> --confirm    # step 4
uds deploy <uds-lab-platform-bundle>.tar.zst --confirm    # KubeVirt+CDI+platform
```

Operator ConfigMap (ADR-0013) will pin DataVolume defaults for this cluster:
`storageClassName: local-path`, `accessModes: [ReadWriteOnce]`,
`volumeMode: Filesystem`.

## 7. Smoke test (after deploy)

```sh
kubectl -n kubevirt get kubevirt kubevirt -o jsonpath='{.status.phase}'   # Deployed
kubectl get crd | grep -E 'kubevirt|cdi'
# create a tiny VMI (no inner uds-core) to prove CDI clone + boot + Service,
# THEN the real uds-core scenario via a LabSession once the operator is deployed.
```
