package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LabSessionPhase is the high-level lifecycle state surfaced to the platform
// server. The operator owns transitions; the server reads them to gate proxying.
type LabSessionPhase string

const (
	// PhaseProvisioning: CR accepted, VM/Service not yet created or not yet running.
	PhaseProvisioning LabSessionPhase = "Provisioning"
	// PhaseRunning: the VM is running but ttyd has not yet answered (phase 1 of
	// two-phase readiness, ADR-0011).
	PhaseRunning LabSessionPhase = "Running"
	// PhaseReady: ttyd answered on :7681; the session is usable and ServiceDNS is set.
	PhaseReady LabSessionPhase = "Ready"
	// PhaseExpired: TTL elapsed; the operator is tearing the session down.
	PhaseExpired LabSessionPhase = "Expired"
	// PhaseFailed: reconciliation hit a terminal error (see Status.Message).
	PhaseFailed LabSessionPhase = "Failed"
)

// LabSessionSpec is the desired session. The server sets every field at create
// time; the spec is immutable thereafter (the operator never mutates spec).
type LabSessionSpec struct {
	// SessionID is the platform's opaque session identifier (also used to derive
	// resource names, e.g. lab-<first8>).
	SessionID string `json:"sessionID"`
	// ScenarioID selects the scenario the operator renders into cloud-init and
	// uses to pick the image tier.
	ScenarioID string `json:"scenarioID"`
	// ClientID binds the session to a browser Client (ADR-0002). The server
	// enforces one active session per Client by listing CRs on this field.
	ClientID string `json:"clientID"`
	// Size is the abstract resource tier (small|medium|large, ADR-0013). Empty
	// means the operator's configured default.
	Size string `json:"size,omitempty"`
	// BrowserEnabled mirrors the scenario's browser flag so the operator can
	// expose the noVNC port and the server can gate the /vnc proxy.
	BrowserEnabled bool `json:"browserEnabled,omitempty"`
	// ExpiresAt is the hard TTL deadline. The operator deletes the session once
	// now > ExpiresAt (replaces the server's in-memory cleanup loop).
	ExpiresAt metav1.Time `json:"expiresAt"`
}

// LabSessionStatus is the observed session, owned entirely by the operator.
type LabSessionStatus struct {
	// Phase is the lifecycle state (see LabSessionPhase).
	Phase LabSessionPhase `json:"phase,omitempty"`
	// ServiceDNS is the in-cluster DNS name of the headless Service fronting the
	// VM (e.g. lab-<id>.uds-lab-vms.svc.cluster.local). The server proxies here
	// instead of to a public IP. Empty until the VM is Running.
	ServiceDNS string `json:"serviceDNS,omitempty"`
	// Message carries human-readable detail, especially for Failed.
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Scenario",type=string,JSONPath=`.spec.scenarioID`
// +kubebuilder:printcolumn:name="Expires",type=string,JSONPath=`.spec.expiresAt`

// LabSession is the lifecycle handle for one running Lab.
type LabSession struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LabSessionSpec   `json:"spec,omitempty"`
	Status LabSessionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LabSessionList is a list of LabSession.
type LabSessionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LabSession `json:"items"`
}
