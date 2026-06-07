package scenario_test

import (
	"context"
	"testing"

	"github.com/enxoco/uds-lab-platform/internal/scenario"
)

// runStoreContract runs the same behavioral assertions against any Store.
// Both FSStore and SQLiteStore must pass every case.
func runStoreContract(t *testing.T, store scenario.Store) {
	t.Helper()
	ctx := context.Background()

	t.Run("List returns summary for each published scenario", func(t *testing.T) {
		summaries, err := store.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(summaries) != 1 {
			t.Fatalf("want 1 summary, got %d", len(summaries))
		}
		if summaries[0].ID != "alpha" {
			t.Errorf("want ID=alpha, got %q", summaries[0].ID)
		}
		if summaries[0].Title != "Alpha Scenario" {
			t.Errorf("want Title=Alpha Scenario, got %q", summaries[0].Title)
		}
	})

	t.Run("Get returns scenario with step content populated", func(t *testing.T) {
		sc, err := store.Get(ctx, "alpha")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if len(sc.Steps) == 0 {
			t.Fatal("want at least one step, got none")
		}
		if sc.Steps[0].Content == "" {
			t.Error("Steps[0].Content must be non-empty")
		}
	})

	t.Run("Get returns error for non-existent ID", func(t *testing.T) {
		_, err := store.Get(ctx, "does-not-exist")
		if err == nil {
			t.Fatal("expected error for non-existent ID, got nil")
		}
	})

	t.Run("GetVMData returns setup.sh content matching fixture", func(t *testing.T) {
		vmd, err := store.GetVMData(ctx, "alpha")
		if err != nil {
			t.Fatalf("GetVMData: %v", err)
		}
		want := "#!/bin/bash\necho setup\n"
		if vmd.SetupSh != want {
			t.Errorf("SetupSh: got %q, want %q", vmd.SetupSh, want)
		}
	})

	t.Run("GetVMData returns verify scripts map with expected filename", func(t *testing.T) {
		vmd, err := store.GetVMData(ctx, "alpha")
		if err != nil {
			t.Fatalf("GetVMData: %v", err)
		}
		if _, ok := vmd.VerifyScripts["step1.sh"]; !ok {
			t.Errorf("VerifyScripts missing 'step1.sh'; got keys: %v", keysOf(vmd.VerifyScripts))
		}
	})

	t.Run("GetVMData returns error for non-existent ID", func(t *testing.T) {
		_, err := store.GetVMData(ctx, "does-not-exist")
		if err == nil {
			t.Fatal("expected error for non-existent ID, got nil")
		}
	})
}

// keysOf returns the keys of a string map for error messages.
func keysOf(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestFSStore_Contract(t *testing.T) {
	runStoreContract(t, scenario.NewFSStore(minimalFS()))
}

func TestSQLiteStore_Contract(t *testing.T) {
	runStoreContract(t, seededSQLiteStore(t))
}
