package session

import "time"

type Status string

const (
	StatusProvisioning Status = "provisioning"
	StatusReady        Status = "ready"
	StatusExpired      Status = "expired"
)

type Session struct {
	ID        string    `json:"id"`
	Scenario  string    `json:"scenario"`
	VMID      int64     `json:"-"`
	VMIP      string    `json:"vm_ip,omitempty"`
	Status    Status    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}
