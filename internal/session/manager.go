// Package session is the thin server-side session layer for the KubeVirt backend
// (Phase E, ADR-0010/0011). Create/Get/Delete operate on LabSession CRs; the
// operator reconciles them into VMIs.
package session

import (
	"context"
	"fmt"
	"io/fs"
	"time"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	labv1 "github.com/enxoco/uds-lab-platform/api/v1alpha1"
	"github.com/enxoco/uds-lab-platform/internal/scenario"
	"github.com/enxoco/uds-lab-platform/internal/sizing"
)

var ErrSessionExists = fmt.Errorf("active session already exists")

// Manager creates/reads/deletes LabSession CRs. The operator owns all VM
// lifecycle; the server only touches the CR.
type Manager struct {
	client      client.Client
	namespace   string
	ttl         time.Duration
	scenariosFS fs.FS
}

// NewManager builds a Manager wired to the cluster.
func NewManager(k8s client.Client, namespace string, ttl time.Duration, scenariosFS fs.FS) *Manager {
	return &Manager{
		client:      k8s,
		namespace:   namespace,
		ttl:         ttl,
		scenariosFS: scenariosFS,
	}
}

// Create enforces one active session per clientID (TOCTOU-safe via LIST then
// CREATE), reads scenario metadata, and creates the LabSession CR.
// Optional extraLabels maps are merged onto the CR labels (later maps win).
func (m *Manager) Create(ctx context.Context, clientID, scenarioID, userEmail string, extraLabels ...map[string]string) (*Session, error) {
	// Reject if a non-terminal session already exists for this client.
	existing := &labv1.LabSessionList{}
	if err := m.client.List(ctx, existing,
		client.InNamespace(m.namespace),
		client.MatchingLabels{"lab.uds.dev/client": clientID},
	); err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	for i := range existing.Items {
		phase := existing.Items[i].Status.Phase
		if phase != labv1.PhaseFailed && phase != labv1.PhaseExpired {
			return nil, ErrSessionExists
		}
	}

	sc, err := scenario.Load(m.scenariosFS, scenarioID)
	if err != nil {
		return nil, fmt.Errorf("scenario %q: %w", scenarioID, err)
	}
	sz, err := sizing.Normalize(sizing.Size(sc.Size))
	if err != nil {
		return nil, fmt.Errorf("invalid size in scenario %q: %w", scenarioID, err)
	}
	id := uuid.New().String()
	now := time.Now()
	expiresAt := now.Add(m.ttl)

	labels := map[string]string{"lab.uds.dev/client": clientID}
	for _, extra := range extraLabels {
		for k, v := range extra {
			labels[k] = v
		}
	}

	ls := &labv1.LabSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: m.namespace,
			Labels:    labels,
		},
		Spec: labv1.LabSessionSpec{
			SessionID:      id,
			ScenarioID:     scenarioID,
			ClientID:       clientID,
			UserEmail:      userEmail,
			Size:           string(sz),
			BrowserEnabled: sc.Browser,
			ExpiresAt:      metav1.NewTime(expiresAt),
		},
	}
	if err := m.client.Create(ctx, ls); err != nil {
		return nil, fmt.Errorf("create LabSession: %w", err)
	}

	return &Session{
		ID:             id,
		Scenario:       scenarioID,
		UserEmail:      userEmail,
		ClientID:       clientID,
		Status:         StatusProvisioning,
		BrowserEnabled: sc.Browser,
		CreatedAt:      now,
		ExpiresAt:      expiresAt,
	}, nil
}

// MarkStepComplete appends a StepRecord to the session's CompletedSteps if the
// step is not already recorded. Records the current time as CompletedAt so the
// CSM dashboard can show real per-step durations.
func (m *Manager) MarkStepComplete(ctx context.Context, id, step string) error {
	ls := &labv1.LabSession{}
	if err := m.client.Get(ctx, client.ObjectKey{Name: id, Namespace: m.namespace}, ls); err != nil {
		return fmt.Errorf("get session %q: %w", id, err)
	}
	for _, s := range ls.Status.CompletedSteps {
		if s.Step == step {
			return nil
		}
	}
	patch := client.MergeFrom(ls.DeepCopy())
	ls.Status.CompletedSteps = append(ls.Status.CompletedSteps, labv1.StepRecord{
		Step:        step,
		CompletedAt: metav1.Now(),
	})
	return m.client.Status().Patch(ctx, ls, patch)
}

// All returns all LabSessions in the manager's namespace.
func (m *Manager) All() []*Session {
	list := &labv1.LabSessionList{}
	if err := m.client.List(context.Background(), list, client.InNamespace(m.namespace)); err != nil {
		return nil
	}
	out := make([]*Session, len(list.Items))
	for i := range list.Items {
		out[i] = lsToSession(&list.Items[i])
	}
	return out
}

// Get reads the current LabSession CR state and maps it to a Session.
func (m *Manager) Get(id string) (*Session, bool) {
	ls := &labv1.LabSession{}
	if err := m.client.Get(context.Background(), client.ObjectKey{
		Name:      id,
		Namespace: m.namespace,
	}, ls); err != nil {
		return nil, false
	}
	return lsToSession(ls), true
}

// GetActive returns the first non-expired session for the given clientID, or
// (nil, false) if none exists. Used by the "resume session" flow on the catalog.
func (m *Manager) GetActive(ctx context.Context, clientID string) (*Session, bool) {
	list := &labv1.LabSessionList{}
	if err := m.client.List(ctx, list,
		client.InNamespace(m.namespace),
		client.MatchingLabels{"lab.uds.dev/client": clientID},
	); err != nil {
		return nil, false
	}
	for i := range list.Items {
		s := lsToSession(&list.Items[i])
		if s.Status != StatusExpired {
			return s, true
		}
	}
	return nil, false
}

// Delete deletes the LabSession CR. Owner references cascade to VMI/Service/NP.
func (m *Manager) Delete(ctx context.Context, id string) error {
	ls := &labv1.LabSession{}
	if err := m.client.Get(ctx, client.ObjectKey{Name: id, Namespace: m.namespace}, ls); err != nil {
		return fmt.Errorf("session %q not found: %w", id, err)
	}
	return m.client.Delete(ctx, ls)
}

// Pause sets Spec.Paused = true, triggering the operator's snapshot-and-suspend flow.
func (m *Manager) Pause(ctx context.Context, id string) error {
	ls := &labv1.LabSession{}
	if err := m.client.Get(ctx, client.ObjectKey{Name: id, Namespace: m.namespace}, ls); err != nil {
		return fmt.Errorf("session %q not found: %w", id, err)
	}
	patch := client.MergeFrom(ls.DeepCopy())
	ls.Spec.Paused = true
	return m.client.Patch(ctx, ls, patch)
}

// Resume sets Spec.Paused = false, triggering the operator to recreate the VM from snapshot.
func (m *Manager) Resume(ctx context.Context, id string) error {
	ls := &labv1.LabSession{}
	if err := m.client.Get(ctx, client.ObjectKey{Name: id, Namespace: m.namespace}, ls); err != nil {
		return fmt.Errorf("session %q not found: %w", id, err)
	}
	patch := client.MergeFrom(ls.DeepCopy())
	ls.Spec.Paused = false
	return m.client.Patch(ctx, ls, patch)
}

func lsStepRecords(in []labv1.StepRecord) []StepRecord {
	if len(in) == 0 {
		return nil
	}
	out := make([]StepRecord, len(in))
	for i, r := range in {
		out[i] = StepRecord{Step: r.Step, CompletedAt: r.CompletedAt.Time}
	}
	return out
}

func lsToSession(ls *labv1.LabSession) *Session {
	status := StatusProvisioning
	switch ls.Status.Phase {
	case labv1.PhaseReady:
		status = StatusReady
	case labv1.PhasePaused:
		status = StatusPaused
	case labv1.PhaseFailed, labv1.PhaseExpired:
		status = StatusExpired
	}
	return &Session{
		ID:             ls.Spec.SessionID,
		Scenario:       ls.Spec.ScenarioID,
		UserEmail:      ls.Spec.UserEmail,
		ClientID:       ls.Spec.ClientID,
		ServiceDNS:     ls.Status.ServiceDNS,
		Status:         status,
		Paused:         ls.Spec.Paused,
		BrowserEnabled: ls.Spec.BrowserEnabled,
		SessionType:    ls.Labels["lab.uds.dev/session-type"],
		AEToken:        ls.Labels["lab.uds.dev/ae-token"],
		CreatedAt:      ls.CreationTimestamp.Time,
		ExpiresAt:      ls.Spec.ExpiresAt.Time,
		CompletedSteps: lsStepRecords(ls.Status.CompletedSteps),
	}
}
