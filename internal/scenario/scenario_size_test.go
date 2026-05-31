package scenario

import (
	"os"
	"testing"

	"github.com/defenseunicorns/uds-lab-platform/internal/sizing"
)

// TestScenarioSizesNormalize loads every bundled scenario and asserts its
// declared size (if any) resolves through sizing.Normalize — i.e. no scenario
// ships an unknown size tier (ADR-0013).
func TestScenarioSizesNormalize(t *testing.T) {
	fsys := os.DirFS("../../scenarios")
	entries, err := os.ReadDir("../../scenarios")
	if err != nil {
		t.Fatalf("read scenarios dir: %v", err)
	}

	var loaded int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sc, err := Load(fsys, e.Name())
		if err != nil {
			t.Errorf("load scenario %q: %v", e.Name(), err)
			continue
		}
		loaded++
		if _, err := sizing.Normalize(sizing.Size(sc.Size)); err != nil {
			t.Errorf("scenario %q has invalid size %q: %v", e.Name(), sc.Size, err)
		}
	}
	if loaded == 0 {
		t.Fatal("no scenarios loaded")
	}
}
