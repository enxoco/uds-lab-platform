package session

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/enxoco/uds-lab-platform/internal/hetzner"
	"github.com/enxoco/uds-lab-platform/internal/scenario"
	"github.com/google/uuid"
)

type VMConfig struct {
	ServerType   string
	Location     string
	Image        string
	SSHKeyNames  []string
	UserDataTmpl *template.Template
	Scenarios    scenario.Store
	InjectPy     string
}

type Manager struct {
	mu             sync.RWMutex
	sessions       map[string]*Session
	clientSessions map[string]string // clientID → sessionID
	hcloud         *hetzner.Client
	ttl            time.Duration
	vmCfg          VMConfig
}

func NewManager(hcloud *hetzner.Client, ttl time.Duration, vmCfg VMConfig) *Manager {
	m := &Manager{
		sessions:       make(map[string]*Session),
		clientSessions: make(map[string]string),
		hcloud:         hcloud,
		ttl:            ttl,
		vmCfg:          vmCfg,
	}
	go m.cleanupLoop()
	return m
}

var ErrSessionExists = fmt.Errorf("active session already exists")

type userDataInput struct {
	SetupSh        string
	VerifyScripts  map[string]string
	BrowserEnabled bool
	InjectPy       string
	SessionID      string
}

func (m *Manager) Create(ctx context.Context, clientID, scenario string) (*Session, error) {
	m.mu.RLock()
	if sid, ok := m.clientSessions[clientID]; ok {
		if _, alive := m.sessions[sid]; alive {
			m.mu.RUnlock()
			return nil, ErrSessionExists
		}
	}
	m.mu.RUnlock()

	vmData, err := m.vmCfg.Scenarios.GetVMData(ctx, scenario)
	if err != nil {
		return nil, fmt.Errorf("load scenario %q: %w", scenario, err)
	}

	browserEnabled := vmData.Browser
	isPlayground := vmData.Playground
	imageOverride := vmData.Image
	serverTypeOverride := vmData.ServerType

	var vmImage string

	var labelSelector string

	if imageOverride != "" {
		labelSelector = "role=uds-lab-playground,tier=" + imageOverride
	} else if isPlayground {
		tier := strings.TrimPrefix(scenario, "playground-")
		labelSelector = "role=uds-lab-playground,tier=" + tier
	} else {
		labelSelector = "role=uds-lab-base"
	}

	found, err := m.hcloud.FindLatestSnapshot(ctx, labelSelector)
	if err != nil {
		return nil, fmt.Errorf("find snapshot for scenario %q with labels %q: %w", scenario, labelSelector, err)
	}

	if found == "" {
		return nil, fmt.Errorf("no snapshot found matching labels %q — build it first with packer/build-images.sh", labelSelector)
	}

	vmImage = found

	log.Printf("create session: scenario=%s image=%s", scenario, vmImage)

	id := uuid.New().String()

	var userData bytes.Buffer
	if err := m.vmCfg.UserDataTmpl.Execute(&userData, userDataInput{
		SetupSh:        vmData.SetupSh,
		VerifyScripts:  vmData.VerifyScripts,
		BrowserEnabled: browserEnabled,
		InjectPy:       m.vmCfg.InjectPy,
		SessionID:      id,
	}); err != nil {
		return nil, fmt.Errorf("render user-data: %w", err)
	}
	now := time.Now()

	serverType := m.vmCfg.ServerType
	if serverTypeOverride != "" {
		serverType = serverTypeOverride
	}

	vmID, vmIP, err := m.hcloud.CreateServer(ctx, hetzner.CreateServerRequest{
		Name:       "lab-" + id[:8],
		ServerType: serverType,
		Image:      vmImage,
		Location:   m.vmCfg.Location,
		UserData:   userData.String(),
		SSHKeys:    m.vmCfg.SSHKeyNames,
	})
	if err != nil {
		return nil, fmt.Errorf("create VM: %w", err)
	}

	s := &Session{
		ID:             id,
		Scenario:       scenario,
		ClientID:       clientID,
		VMID:           vmID,
		VMIP:           vmIP,
		Status:         StatusProvisioning,
		BrowserEnabled: browserEnabled,
		CreatedAt:      now,
		ExpiresAt:      now.Add(m.ttl),
	}

	m.mu.Lock()
	m.sessions[id] = s
	m.clientSessions[clientID] = id
	m.mu.Unlock()

	go m.pollReady(s)
	return s, nil
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *Manager) All() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s)
	}
	return out
}

func (m *Manager) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
		delete(m.clientSessions, s.ClientID)
	}
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("session %q not found", id)
	}
	return m.hcloud.DeleteServer(ctx, s.VMID)
}

// pollReady polls ttyd on the VM until it responds, then marks the session ready.
func (m *Manager) pollReady(s *Session) {
	url := fmt.Sprintf("http://%s:7681/", s.VMIP)
	client := &http.Client{Timeout: 3 * time.Second}

	for {
		time.Sleep(5 * time.Second)

		m.mu.RLock()
		_, alive := m.sessions[s.ID]
		m.mu.RUnlock()
		if !alive {
			return
		}

		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				m.mu.Lock()
				if sess, ok := m.sessions[s.ID]; ok {
					sess.Status = StatusReady
				}
				m.mu.Unlock()
				return
			}
		}
	}
}

func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		m.mu.Lock()
		expired := []*Session{}
		for _, s := range m.sessions {
			if time.Now().After(s.ExpiresAt) {
				expired = append(expired, s)
				delete(m.sessions, s.ID)
			}
		}
		m.mu.Unlock()

		for _, s := range expired {
			m.mu.Lock()
			delete(m.clientSessions, s.ClientID)
			m.mu.Unlock()
			_ = m.hcloud.DeleteServer(context.Background(), s.VMID)
		}
	}
}
