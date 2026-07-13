// Package provider defines the VM backend abstraction the Lab Operator
// reconciles through (ADR-0011). KubeVirt is the only implementation today;
// Azure VMSS is the intended second implementation, which is why lifecycle is
// expressed in provider-neutral terms here.
package provider

import (
	"context"

	labv1 "github.com/enxoco/uds-lab-platform/api/v1alpha1"
)

// Result is the observed outcome of a Reconcile, mapped onto LabSession status.
type Result struct {
	// Phase is the provider's view of VM readiness. The operator may still hold
	// the session in Running until the HTTP readiness probe passes (two-phase
	// readiness), only then promoting to Ready.
	Phase labv1.LabSessionPhase
	// ServiceDNS is the in-cluster DNS name of the Service fronting the VM, set
	// once the VM is far enough along to have one.
	ServiceDNS string
	// Message is human-readable detail, surfaced on failures.
	Message string
}

// Provider creates and destroys the infrastructure backing one LabSession.
// Reconcile must be idempotent: it is called repeatedly until the session is
// Ready (and again on any change), and must converge existing objects rather
// than error on "already exists".
type Provider interface {
	// Reconcile ensures the VM and its supporting objects exist and reports
	// progress. It does not block waiting for readiness.
	Reconcile(ctx context.Context, ls *labv1.LabSession) (Result, error)
	// Teardown removes everything Reconcile created (compute + disk + snapshot).
	// It must be idempotent and succeed if the objects are already gone.
	Teardown(ctx context.Context, ls *labv1.LabSession) error
	// TeardownCompute removes only the VMI, Service, and NetworkPolicy — leaves
	// the DataVolume PVC intact so a snapshot can be taken.
	TeardownCompute(ctx context.Context, ls *labv1.LabSession) error
	// TeardownDisk removes the DataVolume (and therefore its PVC).
	TeardownDisk(ctx context.Context, ls *labv1.LabSession) error
	// Snapshot creates a VolumeSnapshot of the session's disk and returns its
	// name. The snapshot is not ready immediately; poll with SnapshotReady.
	Snapshot(ctx context.Context, ls *labv1.LabSession) (string, error)
	// SnapshotReady returns true when the named VolumeSnapshot is ready to use
	// as a DataVolume source.
	SnapshotReady(ctx context.Context, snapName string) (bool, error)
	// DeleteSnapshot deletes the named VolumeSnapshot. It is idempotent:
	// a not-found error is treated as success.
	DeleteSnapshot(ctx context.Context, snapName string) error
}
