package session

import "time"

type Status string

const (
	StatusProvisioning Status = "provisioning"
	StatusReady        Status = "ready"
	StatusExpired      Status = "expired"
)

type Session struct {
	ID             string    `json:"id"`
	Scenario       string    `json:"scenario"`
	ClientID       string    `json:"-"`
	ServiceDNS     string    `json:"service_dns,omitempty"`
	Status         Status    `json:"status"`
	BrowserEnabled bool      `json:"browser_enabled"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}
