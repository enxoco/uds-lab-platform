package scenario

import (
	"context"
	"fmt"
	"io/fs"

	"gopkg.in/yaml.v3"
)

// Store is the participant-facing scenario data access layer.
// FSStore wraps an fs.FS; SQLiteStore wraps the database.
type Store interface {
	// List returns published scenario summaries for the participant catalog.
	List(ctx context.Context) ([]Summary, error)
	// Get returns a full scenario with step content rendered for the UI.
	Get(ctx context.Context, id string) (*Scenario, error)
	// GetVMData returns files and flags needed to provision a VM.
	GetVMData(ctx context.Context, id string) (*VMData, error)
}

// AdminStore extends Store with authoring operations.
// Only SQLiteStore implements this; FSStore is read-only.
type AdminStore interface {
	Store
	// AdminList returns all scenarios regardless of status.
	AdminList(ctx context.Context) ([]ScenarioRow, error)
	// SetStatus transitions a scenario to draft, published, or archived.
	SetStatus(ctx context.Context, id, status string) error
	// Versions returns the full version history for a scenario, newest first.
	Versions(ctx context.Context, id string) ([]VersionSummary, error)
	// Restore rolls the scenario's files back to a prior version snapshot.
	Restore(ctx context.Context, scenarioID string, versionID int64) error
}

// Compile-time interface checks.
var (
	_ Store      = (*FSStore)(nil)
	_ Store      = (*SQLiteStore)(nil)
	_ AdminStore = (*SQLiteStore)(nil)
)

// VMData holds everything Manager.Create needs to render the user-data template.
type VMData struct {
	SetupSh       string
	VerifyScripts map[string]string // filename → content
	Browser       bool
	Playground    bool
	Image         string
	ServerType    string
}

// FSStore implements Store over an fs.FS (the embedded scenarios directory or a
// SCENARIOS_DIR override on disk). It is read-only and does not implement AdminStore.
type FSStore struct {
	fsys fs.FS
}

func NewFSStore(fsys fs.FS) *FSStore {
	return &FSStore{fsys: fsys}
}

func (s *FSStore) List(_ context.Context) ([]Summary, error) {
	return fsListSummaries(s.fsys)
}

func (s *FSStore) Get(_ context.Context, id string) (*Scenario, error) {
	return fsLoad(s.fsys, id)
}

func (s *FSStore) GetVMData(_ context.Context, id string) (*VMData, error) {
	setupSh, err := fs.ReadFile(s.fsys, id+"/setup.sh")
	if err != nil {
		return nil, fmt.Errorf("scenario %q: setup.sh: %w", id, err)
	}

	verifyScripts := map[string]string{}
	if entries, err := fs.ReadDir(s.fsys, id+"/verify"); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			content, readErr := fs.ReadFile(s.fsys, id+"/verify/"+e.Name())
			if readErr == nil {
				verifyScripts[e.Name()] = string(content)
			}
		}
	}

	d := &VMData{
		SetupSh:       string(setupSh),
		VerifyScripts: verifyScripts,
	}

	if yamlData, err := fs.ReadFile(s.fsys, id+"/scenario.yaml"); err == nil {
		var meta struct {
			Browser    bool   `yaml:"browser"`
			Playground bool   `yaml:"playground"`
			Image      string `yaml:"image"`
			ServerType string `yaml:"serverType"`
		}
		if yaml.Unmarshal(yamlData, &meta) == nil {
			d.Browser = meta.Browser
			d.Playground = meta.Playground
			d.Image = meta.Image
			d.ServerType = meta.ServerType
		}
	}

	return d, nil
}
