package operator

import (
	"testing"

	"github.com/defenseunicorns/uds-lab-platform/internal/sizing"
)

func TestParseAndSizeOverrides(t *testing.T) {
	data := []byte(`
provider: kubevirt
sizes:
  small:
    cpu: "2"
    memory: "4Gi"
  large:
    cpu: "8"
    memory: "16Gi"
images:
  base: registry.local/base@sha256:abc
  uds-core: registry.local/uds-core@sha256:def
`)
	c, err := parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.Provider != "kubevirt" {
		t.Errorf("provider = %q, want kubevirt", c.Provider)
	}
	if c.Images["uds-core"] == "" {
		t.Errorf("missing uds-core image ref")
	}

	overrides, err := c.SizeOverrides()
	if err != nil {
		t.Fatalf("SizeOverrides: %v", err)
	}
	if got := overrides[sizing.Small]; got.CPU != "2" || got.Memory != "4Gi" {
		t.Errorf("small override = %+v", got)
	}
	// medium not configured -> falls back to defaults via sizing.Resolve
	if spec, ok := sizing.Resolve(sizing.Medium, overrides); !ok || spec != sizing.Defaults[sizing.Medium] {
		t.Errorf("medium should resolve to defaults, got %+v ok=%v", spec, ok)
	}
}

func TestSizeOverridesRejectsUnknownTier(t *testing.T) {
	c := &Config{Sizes: map[string]sizeEntry{"xlarge": {CPU: "16", Memory: "32Gi"}}}
	if _, err := c.SizeOverrides(); err == nil {
		t.Fatal("expected error for unknown tier xlarge")
	}
}

func TestLoadMissingPathIsEmpty(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\"): %v", err)
	}
	if len(c.Sizes) != 0 || len(c.Images) != 0 {
		t.Errorf("expected empty config, got %+v", c)
	}
}
