package scenario

import (
	"fmt"
	"io/fs"

	"gopkg.in/yaml.v3"
)

type ServiceLink struct {
	Label string `yaml:"label" json:"label"`
	URL   string `yaml:"url"   json:"url"`
}

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
	Browser     bool          `yaml:"browser"      json:"browser"`
	Playground  bool          `yaml:"playground"   json:"playground"`
	Services    []ServiceLink `yaml:"services"     json:"services"`
}

type Summary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Duration    int    `json:"duration"`
	Difficulty  string `json:"difficulty"`
	Playground  bool   `json:"playground"`
}

func Load(fsys fs.FS, id string) (*Scenario, error) {
	data, err := fs.ReadFile(fsys, id+"/scenario.yaml")
	if err != nil {
		return nil, fmt.Errorf("scenario %q not found: %w", id, err)
	}

	var s Scenario
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse scenario.yaml: %w", err)
	}
	s.ID = id

	for i, step := range s.Steps {
		content, err := fs.ReadFile(fsys, id+"/"+step.Text)
		if err != nil {
			return nil, fmt.Errorf("step %d text file %q: %w", i+1, step.Text, err)
		}
		s.Steps[i].Content = string(content)
		s.Steps[i].HasVerify = step.Verify != ""
	}

	return &s, nil
}

func ListSummaries(fsys fs.FS) ([]Summary, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}

	var out []Summary
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		s, err := Load(fsys, e.Name())
		if err != nil {
			continue
		}
		out = append(out, Summary{
			ID:          s.ID,
			Title:       s.Title,
			Description: s.Description,
			Duration:    s.Duration,
			Difficulty:  s.Difficulty,
			Playground:  s.Playground,
		})
	}
	return out, nil
}
