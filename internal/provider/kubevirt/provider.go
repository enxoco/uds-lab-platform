// Package kubevirt implements provider.Provider on top of KubeVirt VMIs and CDI
// DataVolumes (ADR-0010/0012). One LabSession reconciles to: a DataVolume cloned
// from the scenario's OCI image, a VirtualMachineInstance, a headless Service
// exposing the in-VM ports, and a NetworkPolicy.
//
// SPIKE-DEPENDENT (see plan Phase 0 / Decisions D1–D3): the DataVolume strategy
// (clone-per-session vs golden-PVC vs containerDisk), the storage class/access
// mode, image digests, and confirmation that nested k3d runs inside the VMI are
// all unvalidated until the cluster spike runs. The shapes below are the
// intended design; constants are marked TODO where the spike will set them.
package kubevirt

import (
	"context"
	"fmt"
	"io/fs"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	kvv1 "kubevirt.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	labv1 "github.com/enxoco/uds-lab-platform/api/v1alpha1"
	"github.com/enxoco/uds-lab-platform/internal/cloudinit"
	"github.com/enxoco/uds-lab-platform/internal/provider"
	"github.com/enxoco/uds-lab-platform/internal/scenario"
	"github.com/enxoco/uds-lab-platform/internal/sizing"
)

const (
	// sessionLabel is set on the VMI (and inherited by the virt-launcher pod) so
	// the Service and NetworkPolicy can select it.
	sessionLabel = "lab.uds.dev/session"

	// In-VM ports (unchanged from the Hetzner VM software).
	portInject = 7680 // lab-inject.py (cmd/verify/navigate/services)
	portTTYD   = 7681 // ttyd main (tmux)
	portShell  = 7682 // ttyd direct bash
	portVNC    = 6080 // noVNC/websockify

	// defaultDiskSize is the fallback clone PVC size when GoldenPVCDiskSize is empty.
	defaultDiskSize = "80Gi"
)

// Config wires the provider to the cluster and to scenario content.
type Config struct {
	Client client.Client
	// Namespace holds all VMIs/Services/NetworkPolicies (uds-lab-vms).
	Namespace string
	// ServerNamespace is allowed ingress to the VM Services (the platform server).
	ServerNamespace string

	// UserDataTmpl + ScenariosFS + InjectPy let the operator render cloud-init
	// itself (Decision D1: operator owns scenario content).
	UserDataTmpl *template.Template
	ScenariosFS  fs.FS
	InjectPy     string

	// SizeOverrides come from the operator's ConfigMap (ADR-0013); empty means
	// use sizing.Defaults.
	SizeOverrides map[sizing.Size]sizing.Spec

	// GoldenPVCs maps image tier (base|tools|uds-core) to a golden PVC name.
	// CDI clones one golden PVC per session instead of importing from a registry.
	GoldenPVCs         map[string]string
	GoldenPVCNamespace string // namespace where golden PVCs live; defaults to Namespace
	GoldenPVCDiskSize  string // size of the cloned PVC (must be >= golden PVC size)

	// StorageClass for the cloned DataVolume PVC. Empty uses the cluster default.
	StorageClass string
}

var _ provider.Provider = (*Provider)(nil)

// Provider reconciles LabSessions into KubeVirt objects.
type Provider struct {
	cfg Config
}

// New builds a Provider.
func New(cfg Config) *Provider {
	return &Provider{cfg: cfg}
}

// resourceName derives the shared name for a session's child objects.
func resourceName(ls *labv1.LabSession) string {
	id := ls.Spec.SessionID
	if len(id) > 8 {
		id = id[:8]
	}
	return "lab-" + id
}

// serviceDNS is the in-cluster DNS the server proxies to.
func (p *Provider) serviceDNS(name string) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", name, p.cfg.Namespace)
}

// Reconcile ensures the DataVolume, VMI, Service, and NetworkPolicy exist and
// reports progress. It is idempotent.
func (p *Provider) Reconcile(ctx context.Context, ls *labv1.LabSession) (provider.Result, error) {
	name := resourceName(ls)
	labels := map[string]string{sessionLabel: ls.Spec.SessionID}

	// Resolve golden PVC name + size + cloud-init from scenario content.
	goldenPVCName, err := p.goldenPVCForScenario(ls.Spec.ScenarioID)
	if err != nil {
		return provider.Result{Phase: labv1.PhaseFailed, Message: err.Error()}, nil
	}

	size, err := sizing.Normalize(sizing.Size(ls.Spec.Size))
	if err != nil {
		return provider.Result{Phase: labv1.PhaseFailed, Message: err.Error()}, nil
	}
	spec, ok := sizing.Resolve(size, p.cfg.SizeOverrides)
	if !ok {
		return provider.Result{Phase: labv1.PhaseFailed, Message: fmt.Sprintf("no resource spec for size %q", size)}, nil
	}

	userData, err := cloudinit.Render(p.cfg.UserDataTmpl, p.cfg.ScenariosFS, ls.Spec.ScenarioID, p.cfg.InjectPy, ls.Spec.BrowserEnabled)
	if err != nil {
		return provider.Result{Phase: labv1.PhaseFailed, Message: err.Error()}, nil
	}

	if err := p.ensureDataVolume(ctx, ls, name, labels, goldenPVCName); err != nil {
		return provider.Result{}, fmt.Errorf("ensure datavolume: %w", err)
	}
	if err := p.ensureUserDataSecret(ctx, ls, name, labels, userData); err != nil {
		return provider.Result{}, fmt.Errorf("ensure userdata secret: %w", err)
	}
	if err := p.ensureVMI(ctx, ls, name, labels, spec); err != nil {
		return provider.Result{}, fmt.Errorf("ensure vmi: %w", err)
	}
	if err := p.ensureService(ctx, ls, name, labels); err != nil {
		return provider.Result{}, fmt.Errorf("ensure service: %w", err)
	}
	if err := p.ensureNetworkPolicy(ctx, ls, name, labels); err != nil {
		return provider.Result{}, fmt.Errorf("ensure networkpolicy: %w", err)
	}

	// Read VMI phase. The operator promotes Running -> Ready only after the
	// ttyd HTTP probe passes (two-phase readiness, ADR-0011).
	vmi := &kvv1.VirtualMachineInstance{}
	if err := p.cfg.Client.Get(ctx, client.ObjectKey{Namespace: p.cfg.Namespace, Name: name}, vmi); err != nil {
		if apierrors.IsNotFound(err) {
			return provider.Result{Phase: labv1.PhaseProvisioning}, nil
		}
		return provider.Result{}, fmt.Errorf("get vmi: %w", err)
	}

	switch vmi.Status.Phase {
	case kvv1.Running:
		return provider.Result{Phase: labv1.PhaseRunning, ServiceDNS: p.serviceDNS(name)}, nil
	case kvv1.Failed, kvv1.Succeeded:
		return provider.Result{Phase: labv1.PhaseFailed, Message: fmt.Sprintf("vmi phase %s", vmi.Status.Phase)}, nil
	default:
		return provider.Result{Phase: labv1.PhaseProvisioning}, nil
	}
}

// TeardownCompute deletes the VMI, Service, and NetworkPolicy but leaves the
// DataVolume PVC intact so a VolumeSnapshot can be taken.
func (p *Provider) TeardownCompute(ctx context.Context, ls *labv1.LabSession) error {
	name := resourceName(ls)
	objs := []client.Object{
		&kvv1.VirtualMachineInstance{ObjectMeta: metav1.ObjectMeta{Namespace: p.cfg.Namespace, Name: name}},
		&netv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Namespace: p.cfg.Namespace, Name: name}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: p.cfg.Namespace, Name: name}},
	}
	for _, o := range objs {
		if err := p.cfg.Client.Delete(ctx, o); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete %T %s: %w", o, name, err)
		}
	}
	return nil
}

// TeardownDisk deletes the DataVolume (and therefore its underlying PVC).
func (p *Provider) TeardownDisk(ctx context.Context, ls *labv1.LabSession) error {
	name := resourceName(ls)
	dv := &cdiv1.DataVolume{ObjectMeta: metav1.ObjectMeta{Namespace: p.cfg.Namespace, Name: name}}
	if err := p.cfg.Client.Delete(ctx, dv); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete datavolume %s: %w", name, err)
	}
	return nil
}

// Teardown deletes all session objects: compute, disk, and any snapshot.
func (p *Provider) Teardown(ctx context.Context, ls *labv1.LabSession) error {
	if err := p.TeardownCompute(ctx, ls); err != nil {
		return err
	}
	if err := p.TeardownDisk(ctx, ls); err != nil {
		return err
	}
	if ls.Status.SnapshotName != "" {
		snap := &snapshotv1.VolumeSnapshot{ObjectMeta: metav1.ObjectMeta{
			Namespace: p.cfg.Namespace,
			Name:      ls.Status.SnapshotName,
		}}
		if err := p.cfg.Client.Delete(ctx, snap); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete snapshot %s: %w", ls.Status.SnapshotName, err)
		}
	}
	return nil
}

// Snapshot creates a VolumeSnapshot of the session's disk PVC and returns its
// name. The snapshot is not immediately ready — poll SnapshotReady.
func (p *Provider) Snapshot(ctx context.Context, ls *labv1.LabSession) (string, error) {
	dvName := resourceName(ls)
	snapName := dvName + "-snap"
	vscName := "longhorn-snapshot-vsc"

	snap := &snapshotv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: snapName, Namespace: p.cfg.Namespace},
		Spec: snapshotv1.VolumeSnapshotSpec{
			Source: snapshotv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: ptr(dvName),
			},
			VolumeSnapshotClassName: ptr(vscName),
		},
	}
	if err := p.cfg.Client.Create(ctx, snap); err != nil && !apierrors.IsAlreadyExists(err) {
		return "", fmt.Errorf("create snapshot %s: %w", snapName, err)
	}
	return snapName, nil
}

// SnapshotReady returns true when the named VolumeSnapshot reports ReadyToUse.
func (p *Provider) SnapshotReady(ctx context.Context, snapName string) (bool, error) {
	snap := &snapshotv1.VolumeSnapshot{}
	if err := p.cfg.Client.Get(ctx, client.ObjectKey{Namespace: p.cfg.Namespace, Name: snapName}, snap); err != nil {
		return false, err
	}
	return snap.Status != nil && snap.Status.ReadyToUse != nil && *snap.Status.ReadyToUse, nil
}

// goldenPVCForScenario maps a scenario to its golden PVC name.
// Tier resolution: explicit sc.Image override → playground-<tier> prefix → "base".
func (p *Provider) goldenPVCForScenario(scenarioID string) (string, error) {
	sc, err := scenario.Load(p.cfg.ScenariosFS, scenarioID)
	if err != nil {
		return "", fmt.Errorf("load scenario %q: %w", scenarioID, err)
	}

	tier := "base"
	switch {
	case sc.Image != "":
		tier = sc.Image
	case sc.Playground:
		const prefix = "playground-"
		if len(scenarioID) > len(prefix) && scenarioID[:len(prefix)] == prefix {
			tier = scenarioID[len(prefix):]
		}
	}

	pvcName, ok := p.cfg.GoldenPVCs[tier]
	if !ok || pvcName == "" {
		return "", fmt.Errorf("no golden PVC configured for tier %q (scenario %q)", tier, scenarioID)
	}
	return pvcName, nil
}

func (p *Provider) ensureDataVolume(ctx context.Context, ls *labv1.LabSession, name string, labels map[string]string, goldenPVCName string) error {
	diskSizeStr := p.cfg.GoldenPVCDiskSize
	if diskSizeStr == "" {
		diskSizeStr = defaultDiskSize
	}
	diskQ, err := resource.ParseQuantity(diskSizeStr)
	if err != nil {
		return err
	}

	srcNamespace := p.cfg.GoldenPVCNamespace
	if srcNamespace == "" {
		srcNamespace = p.cfg.Namespace
	}

	var source *cdiv1.DataVolumeSource
	if ls.Status.SnapshotName != "" {
		// Resume from snapshot — restores exact disk state at pause time.
		source = &cdiv1.DataVolumeSource{
			Snapshot: &cdiv1.DataVolumeSourceSnapshot{
				Namespace: p.cfg.Namespace,
				Name:      ls.Status.SnapshotName,
			},
		}
	} else {
		// Fresh session — clone from golden PVC.
		source = &cdiv1.DataVolumeSource{
			PVC: &cdiv1.DataVolumeSourcePVC{
				Namespace: srcNamespace,
				Name:      goldenPVCName,
			},
		}
	}

	dv := &cdiv1.DataVolume{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: p.cfg.Namespace, Labels: labels},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, p.cfg.Client, dv, func() error {
		dv.Labels = labels
		dv.Spec = cdiv1.DataVolumeSpec{
			Source: source,
			PVC: &corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: diskQ},
				},
				StorageClassName: storageClassPtr(p.cfg.StorageClass),
			},
		}
		return controllerutil.SetControllerReference(ls, dv, p.cfg.Client.Scheme())
	})
	return err
}

func (p *Provider) ensureUserDataSecret(ctx context.Context, ls *labv1.LabSession, name string, labels map[string]string, userData string) error {
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: p.cfg.Namespace, Labels: labels}}
	_, err := controllerutil.CreateOrUpdate(ctx, p.cfg.Client, secret, func() error {
		secret.StringData = map[string]string{"userdata": userData}
		return controllerutil.SetControllerReference(ls, secret, p.cfg.Client.Scheme())
	})
	return err
}

func (p *Provider) ensureVMI(ctx context.Context, ls *labv1.LabSession, name string, labels map[string]string, spec sizing.Spec) error {
	cpuQ, err := resource.ParseQuantity(spec.CPU)
	if err != nil {
		return fmt.Errorf("parse cpu %q: %w", spec.CPU, err)
	}
	memQ, err := resource.ParseQuantity(spec.Memory)
	if err != nil {
		return fmt.Errorf("parse memory %q: %w", spec.Memory, err)
	}

	vmi := &kvv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: p.cfg.Namespace, Labels: labels},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, p.cfg.Client, vmi, func() error {
		// VMI spec is effectively immutable once Running; only set on create.
		if vmi.CreationTimestamp.IsZero() {
			vmi.Labels = labels
			vmi.Spec = kvv1.VirtualMachineInstanceSpec{
				Domain: kvv1.DomainSpec{
					Resources: kvv1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    cpuQ,
							corev1.ResourceMemory: memQ,
						},
					},
					Devices: kvv1.Devices{
						Disks: []kvv1.Disk{
							{Name: "rootdisk", DiskDevice: kvv1.DiskDevice{Disk: &kvv1.DiskTarget{Bus: kvv1.DiskBusVirtio}}},
							{Name: "cloudinitdisk", DiskDevice: kvv1.DiskDevice{Disk: &kvv1.DiskTarget{Bus: kvv1.DiskBusVirtio}}},
						},
					},
				},
				Volumes: []kvv1.Volume{
					{Name: "rootdisk", VolumeSource: kvv1.VolumeSource{DataVolume: &kvv1.DataVolumeSource{Name: name}}},
					{Name: "cloudinitdisk", VolumeSource: kvv1.VolumeSource{CloudInitNoCloud: &kvv1.CloudInitNoCloudSource{UserDataSecretRef: &corev1.LocalObjectReference{Name: name}}}},
				},
			}
		}
		return controllerutil.SetControllerReference(ls, vmi, p.cfg.Client.Scheme())
	})
	return err
}

func (p *Provider) ensureService(ctx context.Context, ls *labv1.LabSession, name string, labels map[string]string) error {
	ports := []corev1.ServicePort{
		{Name: "inject", Port: portInject, TargetPort: intstr.FromInt(portInject)},
		{Name: "ttyd", Port: portTTYD, TargetPort: intstr.FromInt(portTTYD)},
		{Name: "shell", Port: portShell, TargetPort: intstr.FromInt(portShell)},
	}
	if ls.Spec.BrowserEnabled {
		ports = append(ports, corev1.ServicePort{Name: "vnc", Port: portVNC, TargetPort: intstr.FromInt(portVNC)})
	}

	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: p.cfg.Namespace, Labels: labels}}
	_, err := controllerutil.CreateOrUpdate(ctx, p.cfg.Client, svc, func() error {
		svc.Labels = labels
		svc.Spec.ClusterIP = corev1.ClusterIPNone // headless: resolves to the launcher pod IP
		svc.Spec.Selector = labels
		svc.Spec.Ports = ports
		return controllerutil.SetControllerReference(ls, svc, p.cfg.Client.Scheme())
	})
	return err
}

func (p *Provider) ensureNetworkPolicy(ctx context.Context, ls *labv1.LabSession, name string, labels map[string]string) error {
	tcp := corev1.ProtocolTCP
	udp := corev1.ProtocolUDP
	dns := intstr.FromInt(53)

	ingressPorts := []netv1.NetworkPolicyPort{
		{Protocol: &tcp, Port: ptr(intstr.FromInt(portInject))},
		{Protocol: &tcp, Port: ptr(intstr.FromInt(portTTYD))},
		{Protocol: &tcp, Port: ptr(intstr.FromInt(portShell))},
	}
	if ls.Spec.BrowserEnabled {
		ingressPorts = append(ingressPorts, netv1.NetworkPolicyPort{Protocol: &tcp, Port: ptr(intstr.FromInt(portVNC))})
	}

	np := &netv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: p.cfg.Namespace, Labels: labels}}
	_, err := controllerutil.CreateOrUpdate(ctx, p.cfg.Client, np, func() error {
		np.Labels = labels
		np.Spec = netv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: labels},
			PolicyTypes: []netv1.PolicyType{netv1.PolicyTypeIngress, netv1.PolicyTypeEgress},
			// Ingress: only the platform server's namespace may reach the VM ports
			// (Phase E's proxy depends on this). VMI<->VMI is denied by omission.
			Ingress: []netv1.NetworkPolicyIngressRule{{
				From: []netv1.NetworkPolicyPeer{{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"kubernetes.io/metadata.name": p.cfg.ServerNamespace},
					},
				}},
				Ports: ingressPorts,
			}},
			// Egress: DNS + unrestricted internet for lab VM workloads.
			// VMs run arbitrary student/exercise code that installs packages,
			// hits APIs, and runs k3d clusters — full outbound is intentional.
			Egress: []netv1.NetworkPolicyEgressRule{
				{
					Ports: []netv1.NetworkPolicyPort{
						{Protocol: &udp, Port: &dns},
						{Protocol: &tcp, Port: &dns},
					},
				},
				{}, // allow all other egress
			},
		}
		return controllerutil.SetControllerReference(ls, np, p.cfg.Client.Scheme())
	})
	return err
}

func storageClassPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ptr[T any](v T) *T { return &v }
