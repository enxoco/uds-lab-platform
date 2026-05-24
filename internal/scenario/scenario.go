package scenario

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Step struct {
	Title     string `yaml:"title"  json:"title"`
	Text      string `yaml:"text"   json:"-"`
	Verify    string `yaml:"verify" json:"-"`
	Content   string `yaml:"-"      json:"content"`
	HasVerify bool   `yaml:"-"      json:"has_verify"`
}

type Scenario struct {
	ID          string   `yaml:"-"            json:"id"`
	Title       string   `yaml:"title"        json:"title"`
	Description string   `yaml:"description"  json:"description"`
	Duration    int      `yaml:"duration"     json:"duration"`
	Difficulty  string   `yaml:"difficulty"   json:"difficulty"`
	Tags        []string `yaml:"tags"         json:"tags"`
	Steps       []Step   `yaml:"steps"        json:"steps"`
}

type Summary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Duration    int    `json:"duration"`
	Difficulty  string `json:"difficulty"`
}

func Load(scenariosDir, id string) (*Scenario, error) {
	base := filepath.Join(scenariosDir, id)

	data, err := os.ReadFile(filepath.Join(base, "scenario.yaml"))
	if err != nil {
		return nil, fmt.Errorf("scenario %q not found: %w", id, err)
	}

	var s Scenario
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse scenario.yaml: %w", err)
	}
	s.ID = id

	for i, step := range s.Steps {
		content, err := os.ReadFile(filepath.Join(base, step.Text))
		if err != nil {
			return nil, fmt.Errorf("step %d text file %q: %w", i+1, step.Text, err)
		}
		s.Steps[i].Content = string(content)
		s.Steps[i].HasVerify = step.Verify != ""
	}

	return &s, nil
}

func ListSummaries(scenariosDir string) ([]Summary, error) {
	entries, err := os.ReadDir(scenariosDir)
	if err != nil {
		return nil, err
	}

	var out []Summary
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		s, err := Load(scenariosDir, e.Name())
		if err != nil {
			continue
		}
		out = append(out, Summary{
			ID:          s.ID,
			Title:       s.Title,
			Description: s.Description,
			Duration:    s.Duration,
			Difficulty:  s.Difficulty,
		})
	}
	return out, nil
}
