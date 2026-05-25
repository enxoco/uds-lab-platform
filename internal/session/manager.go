package session

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"sync"
	"text/template"
	"time"

	"github.com/defenseunicorns/uds-lab-platform/internal/hetzner"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type VMConfig struct {
	ServerType   string
	Location     string
	Image        string
	SSHKeyNames  []string
	UserDataTmpl *template.Template
	ScenariosFS  fs.FS
}

type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	hcloud   *hetzner.Client
	ttl      time.Duration
	vmCfg    VMConfig
}

func NewManager(hcloud *hetzner.Client, ttl time.Duration, vmCfg VMConfig) *Manager {
	m := &Manager{
		sessions: make(map[string]*Session),
		hcloud:   hcloud,
		ttl:      ttl,
		vmCfg:    vmCfg,
	}
	go m.cleanupLoop()
	return m
}

type userDataInput struct {
	SetupSh        string
	VerifyScripts  map[string]string
	BrowserEnabled bool
}

func (m *Manager) Create(ctx context.Context, scenario string) (*Session, error) {
	setupSh, err := fs.ReadFile(m.vmCfg.ScenariosFS, scenario+"/setup.sh")
	if err != nil {
		return nil, fmt.Errorf("scenario %q not found: %w", scenario, err)
	}

	// Read flags from scenario.yaml
	browserEnabled := false
	isPlayground := false
	if yamlData, err := fs.ReadFile(m.vmCfg.ScenariosFS, scenario+"/scenario.yaml"); err == nil {
		var meta struct {
			Browser    bool `yaml:"browser"`
			Playground bool `yaml:"playground"`
		}
		if yaml.Unmarshal(yamlData, &meta) == nil {
			browserEnabled = meta.Browser
			isPlayground = meta.Playground
		}
	}

	vmImage := m.vmCfg.Image
	if isPlayground {
		if found, err := m.hcloud.FindLatestSnapshot(ctx, "uds-lab-"+scenario); err == nil && found != "" {
			vmImage = found
		}
	}

	verifyScripts := map[string]string{}
	if entries, err := fs.ReadDir(m.vmCfg.ScenariosFS, scenario+"/verify"); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			content, err := fs.ReadFile(m.vmCfg.ScenariosFS, scenario+"/verify/"+e.Name())
			if err == nil {
				verifyScripts[e.Name()] = string(content)
			}
		}
	}

	var userData bytes.Buffer
	if err := m.vmCfg.UserDataTmpl.Execute(&userData, userDataInput{
		SetupSh:        string(setupSh),
		VerifyScripts:  verifyScripts,
		BrowserEnabled: browserEnabled,
	}); err != nil {
		return nil, fmt.Errorf("render user-data: %w", err)
	}

	id := uuid.New().String()
	now := time.Now()

	vmID, vmIP, err := m.hcloud.CreateServer(ctx, hetzner.CreateServerRequest{
		Name:       "lab-" + id[:8],
		ServerType: m.vmCfg.ServerType,
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
		VMID:           vmID,
		VMIP:           vmIP,
		Status:         StatusProvisioning,
		BrowserEnabled: browserEnabled,
		CreatedAt:      now,
		ExpiresAt:      now.Add(m.ttl),
	}

	m.mu.Lock()
	m.sessions[id] = s
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

func (m *Manager) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
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
			resp.Body.Close()
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
			_ = m.hcloud.DeleteServer(context.Background(), s.VMID)
		}
	}
}
