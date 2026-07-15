package controller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	labv1 "github.com/enxoco/uds-lab-platform/api/v1alpha1"
	"github.com/enxoco/uds-lab-platform/internal/provider"
)

type testProvider struct {
	reconcile func(context.Context, *labv1.LabSession) (provider.Result, error)
}

func (p testProvider) Reconcile(ctx context.Context, ls *labv1.LabSession) (provider.Result, error) {
	if p.reconcile == nil {
		return provider.Result{}, nil
	}
	return p.reconcile(ctx, ls)
}

func (testProvider) Teardown(context.Context, *labv1.LabSession) error        { return nil }
func (testProvider) TeardownCompute(context.Context, *labv1.LabSession) error { return nil }
func (testProvider) TeardownDisk(context.Context, *labv1.LabSession) error    { return nil }
func (testProvider) Snapshot(context.Context, *labv1.LabSession) (string, error) {
	return "", nil
}
func (testProvider) SnapshotReady(context.Context, string) (bool, error) { return false, nil }
func (testProvider) DeleteSnapshot(context.Context, string) error        { return nil }

func controllerTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := labv1.AddToScheme(s); err != nil {
		t.Fatalf("add LabSession scheme: %v", err)
	}
	return s
}

func TestReconcileFinalizerWaitsForWatchEvent(t *testing.T) {
	ctx := context.Background()
	s := controllerTestScheme(t)
	ls := &labv1.LabSession{ObjectMeta: metav1.ObjectMeta{Name: "session", Namespace: "uds-lab-vms"}}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(ls).Build()
	r := &LabSessionReconciler{Client: c, Scheme: s, Provider: testProvider{}}

	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
		Name: ls.Name, Namespace: ls.Namespace,
	}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Fatalf("finalizer update must wait for its watch event, got requeue result %+v", result)
	}

	got := &labv1.LabSession{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(ls), got); err != nil {
		t.Fatalf("get LabSession: %v", err)
	}
	if !containsString(got.Finalizers, finalizer) {
		t.Fatalf("finalizer %q was not added", finalizer)
	}
}

func TestReconcileLifecycleStatusPreservesConcurrentStepUpdate(t *testing.T) {
	ctx := context.Background()
	s := controllerTestScheme(t)
	ls := &labv1.LabSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "session",
			Namespace:  "uds-lab-vms",
			Finalizers: []string{finalizer},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&labv1.LabSession{}).
		WithObjects(ls).
		Build()

	p := testProvider{reconcile: func(ctx context.Context, _ *labv1.LabSession) (provider.Result, error) {
		latest := &labv1.LabSession{}
		if err := c.Get(ctx, client.ObjectKeyFromObject(ls), latest); err != nil {
			return provider.Result{}, err
		}
		before := latest.DeepCopy()
		latest.Status.CompletedSteps = append(latest.Status.CompletedSteps, labv1.StepRecord{Step: "verify"})
		if err := c.Status().Patch(ctx, latest, client.MergeFrom(before)); err != nil {
			return provider.Result{}, err
		}
		return provider.Result{Phase: labv1.PhaseProvisioning, Message: "creating VM"}, nil
	}}
	r := &LabSessionReconciler{Client: c, Scheme: s, Provider: p}

	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{
		Name: ls.Name, Namespace: ls.Namespace,
	}})
	if err != nil {
		t.Fatalf("reconcile raced concurrent status writer: %v", err)
	}

	got := &labv1.LabSession{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(ls), got); err != nil {
		t.Fatalf("get LabSession: %v", err)
	}
	if got.Status.Phase != labv1.PhaseProvisioning || got.Status.Message != "creating VM" {
		t.Fatalf("lifecycle status not applied: %+v", got.Status)
	}
	if len(got.Status.CompletedSteps) != 1 || got.Status.CompletedSteps[0].Step != "verify" {
		t.Fatalf("concurrent completed step was lost: %+v", got.Status.CompletedSteps)
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
