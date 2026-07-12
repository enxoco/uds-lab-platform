// Package pod implements provider.Provider using plain Kubernetes Pods instead
// of KubeVirt VMIs. It is the local-development / macOS backend: any cluster
// that can schedule containers (OrbStack, Rancher Desktop, kind, minikube) can
// run it without nested virtualisation.
//
// Each LabSession maps to:
//   - A Pod running the lab container image, with an init-container that runs
//     setup.sh for the scenario and writes artefacts to a shared emptyDir.
//   - A headless Service on the same ports the KubeVirt provider exposes.
//   - A NetworkPolicy with identical ingress/egress rules.
//
// Pause/resume are not supported (Snapshot/SnapshotReady/DeleteSnapshot all
// return ErrNotSupported). The controller handles this gracefully: it only calls
// those methods after Spec.Paused is set, which the lab server guards behind the
// session owner check — so on the dev backend users will see an error if they
// try to pause, which is acceptable.
package pod

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	labv1 "github.com/enxoco/uds-lab-platform/api/v1alpha1"
	"github.com/enxoco/uds-lab-platform/internal/provider"
	"github.com/enxoco/uds-lab-platform/internal/scenario"
	"github.com/enxoco/uds-lab-platform/internal/sizing"
)

// ErrNotSupported is returned by snapshot operations, which have no meaningful
// implementation for pod-based sessions.
var ErrNotSupported = errors.New("pod provider: operation not supported (dev backend)")

const (
	sessionLabel = "lab.uds.dev/session"

	// Port constants mirror the kubevirt provider exactly so the same proxy
	// and readiness-probe logic in the controller works unchanged.
	portInject = 7680
	portTTYD   = 7681
	portShell  = 7682
	portVNC    = 6080
)

// Config wires the pod provider to the cluster.
type Config struct {
	Client client.Client
	// Namespace where Pods/Services/NetworkPolicies are created.
	Namespace string
	// ServerNamespace is allowed ingress by the NetworkPolicy.
	ServerNamespace string
	// ScenariosFS is used to load scenario metadata (setup.sh, verify scripts).
	ScenariosFS fs.FS
	// Image is the container image that runs the lab environment.
	// It must expose ttyd on :7681, a bash ttyd on :7682, and lab-inject.py on :7680.
	// Example: ghcr.io/enxoco/uds-lab-vm:latest
	Image string
	// ImagePullPolicy defaults to IfNotPresent.
	ImagePullPolicy corev1.PullPolicy
	// SizeOverrides optionally maps size tiers to CPU/memory; uses sizing.Defaults
	// when empty.
	SizeOverrides map[sizing.Size]sizing.Spec
}

var _ provider.Provider = (*Provider)(nil)

// Provider reconciles LabSessions into Pods.
type Provider struct {
	cfg Config
}

// New builds a Provider.
func New(cfg Config) *Provider {
	if cfg.ImagePullPolicy == "" {
		cfg.ImagePullPolicy = corev1.PullIfNotPresent
	}
	return &Provider{cfg: cfg}
}

func resourceName(ls *labv1.LabSession) string {
	id := ls.Spec.SessionID
	if len(id) > 8 {
		id = id[:8]
	}
	return "lab-" + id
}

func (p *Provider) serviceDNS(name string) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", name, p.cfg.Namespace)
}

// Reconcile ensures the Pod, Service, and NetworkPolicy exist and reports progress.
func (p *Provider) Reconcile(ctx context.Context, ls *labv1.LabSession) (provider.Result, error) {
	name := resourceName(ls)
	labels := map[string]string{sessionLabel: ls.Spec.SessionID}

	size, err := sizing.Normalize(sizing.Size(ls.Spec.Size))
	if err != nil {
		return provider.Result{Phase: labv1.PhaseFailed, Message: err.Error()}, nil
	}
	spec, ok := sizing.Resolve(size, p.cfg.SizeOverrides)
	if !ok {
		return provider.Result{Phase: labv1.PhaseFailed, Message: fmt.Sprintf("no resource spec for size %q", size)}, nil
	}

	sc, err := scenario.Load(p.cfg.ScenariosFS, ls.Spec.ScenarioID)
	if err != nil {
		return provider.Result{Phase: labv1.PhaseFailed, Message: err.Error()}, nil
	}

	if err := p.ensurePod(ctx, ls, name, labels, spec, sc); err != nil {
		return provider.Result{}, fmt.Errorf("ensure pod: %w", err)
	}
	if err := p.ensureService(ctx, ls, name, labels); err != nil {
		return provider.Result{}, fmt.Errorf("ensure service: %w", err)
	}
	if err := p.ensureNetworkPolicy(ctx, ls, name, labels); err != nil {
		return provider.Result{}, fmt.Errorf("ensure networkpolicy: %w", err)
	}

	pod := &corev1.Pod{}
	if err := p.cfg.Client.Get(ctx, client.ObjectKey{Namespace: p.cfg.Namespace, Name: name}, pod); err != nil {
		if apierrors.IsNotFound(err) {
			return provider.Result{Phase: labv1.PhaseProvisioning}, nil
		}
		return provider.Result{}, fmt.Errorf("get pod: %w", err)
	}

	switch pod.Status.Phase {
	case corev1.PodRunning:
		// The pod's ReadinessProbe already validates that ttyd is answering on
		// :7681, so when all containers are Ready we can promote directly to
		// PhaseReady — bypassing the controller's external HTTP probe which
		// cannot reach in-cluster DNS when the operator runs outside the cluster.
		for _, cs := range pod.Status.ContainerStatuses {
			if !cs.Ready {
				return provider.Result{Phase: labv1.PhaseProvisioning}, nil
			}
		}
		return provider.Result{Phase: labv1.PhaseReady, ServiceDNS: p.serviceDNS(name)}, nil
	case corev1.PodFailed, corev1.PodSucceeded:
		return provider.Result{Phase: labv1.PhaseFailed, Message: fmt.Sprintf("pod phase %s", pod.Status.Phase)}, nil
	default:
		return provider.Result{Phase: labv1.PhaseProvisioning}, nil
	}
}

// Teardown deletes the Pod, Service, and NetworkPolicy.
func (p *Provider) Teardown(ctx context.Context, ls *labv1.LabSession) error {
	name := resourceName(ls)
	objs := []client.Object{
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: p.cfg.Namespace, Name: name}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: p.cfg.Namespace, Name: name}},
		&netv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Namespace: p.cfg.Namespace, Name: name}},
	}
	for _, o := range objs {
		if err := p.cfg.Client.Delete(ctx, o); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete %T %s: %w", o, name, err)
		}
	}
	return nil
}

// TeardownCompute and TeardownDisk are equivalent for the pod backend (no
// separate disk object), so both delegate to Teardown.
func (p *Provider) TeardownCompute(ctx context.Context, ls *labv1.LabSession) error {
	return p.Teardown(ctx, ls)
}

func (p *Provider) TeardownDisk(ctx context.Context, ls *labv1.LabSession) error {
	return nil // no separate disk to clean up
}

// Snapshot, SnapshotReady, and DeleteSnapshot are not supported by the pod
// backend. Pause/resume requires the KubeVirt provider.
func (p *Provider) Snapshot(_ context.Context, _ *labv1.LabSession) (string, error) {
	return "", ErrNotSupported
}

func (p *Provider) SnapshotReady(_ context.Context, _ string) (bool, error) {
	return false, ErrNotSupported
}

func (p *Provider) DeleteSnapshot(_ context.Context, _ string) error {
	return ErrNotSupported
}

// ensurePod creates or updates the session Pod. Pod specs are immutable once
// Running so we only set the spec on creation (mirroring ensureVMI).
func (p *Provider) ensurePod(ctx context.Context, ls *labv1.LabSession, name string, labels map[string]string, spec sizing.Spec, sc *scenario.Scenario) error {
	cpuQ, err := resource.ParseQuantity(spec.CPU)
	if err != nil {
		return fmt.Errorf("parse cpu %q: %w", spec.CPU, err)
	}
	memQ, err := resource.ParseQuantity(spec.Memory)
	if err != nil {
		return fmt.Errorf("parse memory %q: %w", spec.Memory, err)
	}

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: p.cfg.Namespace, Labels: labels}}
	_, err = controllerutil.CreateOrUpdate(ctx, p.cfg.Client, pod, func() error {
		if !pod.CreationTimestamp.IsZero() {
			// Pod spec is immutable after creation; only refresh labels.
			pod.Labels = labels
			return controllerutil.SetControllerReference(ls, pod, p.cfg.Client.Scheme())
		}
		pod.Labels = labels
		pod.Spec = corev1.PodSpec{
			// Restart on container crash so transient failures self-heal.
			RestartPolicy: corev1.RestartPolicyAlways,

			// Init container: runs setup.sh for the scenario, writes any
			// generated artefacts into /lab-work (emptyDir shared with main).
			InitContainers: []corev1.Container{
				{
					Name:            "setup",
					Image:           p.cfg.Image,
					ImagePullPolicy: p.cfg.ImagePullPolicy,
					Command:         []string{"/bin/bash", "-c", fmt.Sprintf("/lab/scenarios/%s/setup.sh 2>&1 | tee /lab-work/setup.log", sc.ID)},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "lab-work", MountPath: "/lab-work"},
					},
					Env: []corev1.EnvVar{
						{Name: "SCENARIO_ID", Value: sc.ID},
						{Name: "LAB_WORK_DIR", Value: "/lab-work"},
					},
				},
			},

			// Main container: runs ttyd + lab-inject.py, mirrors the VM's
			// long-running process set.
			Containers: []corev1.Container{
				{
					Name:            "lab",
					Image:           p.cfg.Image,
					ImagePullPolicy: p.cfg.ImagePullPolicy,
					// Entrypoint is defined in the Dockerfile; the container
					// starts ttyd on :7681/:7682 and lab-inject.py on :7680.
					Ports: []corev1.ContainerPort{
						{Name: "inject", ContainerPort: portInject, Protocol: corev1.ProtocolTCP},
						{Name: "ttyd", ContainerPort: portTTYD, Protocol: corev1.ProtocolTCP},
						{Name: "shell", ContainerPort: portShell, Protocol: corev1.ProtocolTCP},
						{Name: "vnc", ContainerPort: portVNC, Protocol: corev1.ProtocolTCP},
					},
					Env: []corev1.EnvVar{
						{Name: "SCENARIO_ID", Value: sc.ID},
						{Name: "LAB_WORK_DIR", Value: "/lab-work"},
						{Name: "BROWSER_ENABLED", Value: boolEnv(ls.Spec.BrowserEnabled)},
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    cpuQ,
							corev1.ResourceMemory: memQ,
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "lab-work", MountPath: "/lab-work"},
					},
					// Readiness probe matches the controller's HTTP probe so
					// the two-phase readiness check (Running → Ready) aligns.
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/",
								Port: intstr.FromInt(portTTYD),
							},
						},
						InitialDelaySeconds: 5,
						PeriodSeconds:       5,
					},
				},
			},

			Volumes: []corev1.Volume{
				// Shared scratch space between init and main containers.
				{Name: "lab-work", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
		}
		return controllerutil.SetControllerReference(ls, pod, p.cfg.Client.Scheme())
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
		svc.Spec.ClusterIP = corev1.ClusterIPNone
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
			Ingress: []netv1.NetworkPolicyIngressRule{{
				From: []netv1.NetworkPolicyPeer{{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"kubernetes.io/metadata.name": p.cfg.ServerNamespace},
					},
				}},
				Ports: ingressPorts,
			}},
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

func boolEnv(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func ptr[T any](v T) *T { return &v }
