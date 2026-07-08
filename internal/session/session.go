package session

import "time"

type Status string

const (
	StatusProvisioning Status = "provisioning"
	StatusReady        Status = "ready"
	StatusExpired      Status = "expired"
)

// StepRecord mirrors labv1.StepRecord using plain time.Time.
type StepRecord struct {
	Step        string    `json:"step"`
	CompletedAt time.Time `json:"completed_at"`
}

type Session struct {
	ID             string       `json:"id"`
	Scenario       string       `json:"scenario"`
	UserEmail      string       `json:"user_email,omitempty"`
	ClientID       string       `json:"-"`
	ServiceDNS     string       `json:"service_dns,omitempty"`
	Status         Status       `json:"status"`
	BrowserEnabled bool         `json:"browser_enabled"`
	SessionType    string       `json:"session_type,omitempty"`
	AEToken        string       `json:"-"`
	CreatedAt      time.Time    `json:"created_at"`
	ExpiresAt      time.Time    `json:"expires_at"`
	CompletedSteps []StepRecord `json:"completed_steps,omitempty"`
}
