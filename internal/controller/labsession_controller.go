// Package controller holds the LabSession reconciler (ADR-0011). It is
// provider-agnostic: it owns the lifecycle state machine (provisioning →
// running → ready, TTL expiry, teardown) and delegates VM creation/deletion to
// a provider.Provider.
package controller

import (
	"context"
	"net/http"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	labv1 "github.com/enxoco/uds-lab-platform/api/v1alpha1"
	"github.com/enxoco/uds-lab-platform/internal/provider"
)

const finalizer = "lab.uds.dev/teardown"

// requeueWhilePending is how often we re-check a not-yet-ready session (covers
// VMI phase transitions and the ttyd readiness probe).
const requeueWhilePending = 5 * time.Second

// LabSessionReconciler reconciles LabSession objects.
type LabSessionReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Provider provider.Provider
	// Probe checks ttyd readiness over the Service DNS (phase 2 of two-phase
	// readiness). Injectable for tests; defaults to an HTTP GET on :7681.
	Probe func(ctx context.Context, serviceDNS string) bool
}

// +kubebuilder:rbac:groups=lab.uds.dev,resources=labsessions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=lab.uds.dev,resources=labsessions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=lab.uds.dev,resources=labsessions/finalizers,verbs=update
// +kubebuilder:rbac:groups=kubevirt.io,resources=virtualmachineinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cdi.kubevirt.io,resources=datavolumes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete

// Reconcile drives one LabSession toward Ready, enforces its TTL, and tears it
// down on deletion.
func (r *LabSessionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	ls := &labv1.LabSession{}
	if err := r.Get(ctx, req.NamespacedName, ls); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Deletion: run teardown via finalizer.
	if !ls.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ls, finalizer) {
			if err := r.Provider.Teardown(ctx, ls); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(ls, finalizer)
			if err := r.Update(ctx, ls); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Ensure finalizer so teardown runs before the CR disappears.
	if !controllerutil.ContainsFinalizer(ls, finalizer) {
		controllerutil.AddFinalizer(ls, finalizer)
		if err := r.Update(ctx, ls); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// TTL: once expired, tear down the VM but retain the CR so the CSM
	// dashboard can read completed steps for up to 30 days.
	if !ls.Spec.ExpiresAt.IsZero() && time.Now().After(ls.Spec.ExpiresAt.Time) {
		if ls.Status.Phase != labv1.PhaseExpired {
			l.Info("session expired, tearing down VM", "session", ls.Spec.SessionID)
			if err := r.Provider.Teardown(ctx, ls); err != nil {
				return ctrl.Result{}, err
			}
			ls.Status.Phase = labv1.PhaseExpired
			if err := r.Status().Update(ctx, ls); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Converge infrastructure.
	res, err := r.Provider.Reconcile(ctx, ls)
	if err != nil {
		return ctrl.Result{}, err
	}

	phase := res.Phase
	// Two-phase readiness: VM Running + ttyd answering => Ready.
	if res.Phase == labv1.PhaseRunning && res.ServiceDNS != "" && r.probe(ctx, res.ServiceDNS) {
		phase = labv1.PhaseReady
	}

	if ls.Status.Phase != phase || ls.Status.ServiceDNS != res.ServiceDNS || ls.Status.Message != res.Message {
		ls.Status.Phase = phase
		ls.Status.ServiceDNS = res.ServiceDNS
		ls.Status.Message = res.Message
		if err := r.Status().Update(ctx, ls); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Requeue until Ready (and to re-check TTL).
	if phase != labv1.PhaseReady {
		return ctrl.Result{RequeueAfter: requeueWhilePending}, nil
	}
	return ctrl.Result{RequeueAfter: requeueUntilExpiry(ls)}, nil
}

func (r *LabSessionReconciler) probe(ctx context.Context, serviceDNS string) bool {
	if r.Probe != nil {
		return r.Probe(ctx, serviceDNS)
	}
	return defaultProbe(ctx, serviceDNS)
}

func defaultProbe(ctx context.Context, serviceDNS string) bool {
	url := "http://" + serviceDNS + ":7681/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// requeueUntilExpiry returns when to next reconcile a Ready session so TTL is
// enforced promptly without busy-looping.
func requeueUntilExpiry(ls *labv1.LabSession) time.Duration {
	if ls.Spec.ExpiresAt.IsZero() {
		return time.Minute
	}
	d := time.Until(ls.Spec.ExpiresAt.Time)
	if d < time.Second {
		return time.Second
	}
	return d
}

// SetupWithManager registers the reconciler.
func (r *LabSessionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&labv1.LabSession{}).
		Complete(r)
}
