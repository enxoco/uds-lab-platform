package scenario_test

import (
	"context"
	"testing"

	"github.com/enxoco/uds-lab-platform/internal/scenario"
)

func TestSeedOnce_EmptyDB_ScenariosRowPublished(t *testing.T) {
	db := newMemDB(t)
	ctx := context.Background()

	if err := scenario.SeedOnce(ctx, db, minimalFS()); err != nil {
		t.Fatalf("SeedOnce: %v", err)
	}

	var status string
	err := db.QueryRowContext(ctx, `SELECT status FROM scenarios WHERE id='alpha'`).Scan(&status)
	if err != nil {
		t.Fatalf("query scenarios: %v", err)
	}
	if status != "published" {
		t.Errorf("want status=published, got %q", status)
	}
}

func TestSeedOnce_Idempotency_SecondCallIsNoop(t *testing.T) {
	db := newMemDB(t)
	ctx := context.Background()

	if err := scenario.SeedOnce(ctx, db, minimalFS()); err != nil {
		t.Fatalf("first SeedOnce: %v", err)
	}

	var countBefore int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM scenarios`).Scan(&countBefore); err != nil {
		t.Fatalf("count before: %v", err)
	}

	if err := scenario.SeedOnce(ctx, db, minimalFS()); err != nil {
		t.Fatalf("second SeedOnce: %v", err)
	}

	var countAfter int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM scenarios`).Scan(&countAfter); err != nil {
		t.Fatalf("count after: %v", err)
	}

	if countAfter != countBefore {
		t.Errorf("second SeedOnce changed row count: before=%d after=%d", countBefore, countAfter)
	}
}

func TestSeedOnce_Idempotency_FlagPresentRowsDeleted_SecondCallNil(t *testing.T) {
	db := newMemDB(t)
	ctx := context.Background()

	// First seed — writes the seeded flag and rows.
	if err := scenario.SeedOnce(ctx, db, minimalFS()); err != nil {
		t.Fatalf("first SeedOnce: %v", err)
	}

	// Manually delete all scenario rows to simulate a corrupted state.
	if _, err := db.ExecContext(ctx, `DELETE FROM scenarios`); err != nil {
		t.Fatalf("delete scenarios: %v", err)
	}

	// Second call — flag is present, so it must return nil without re-seeding.
	if err := scenario.SeedOnce(ctx, db, minimalFS()); err != nil {
		t.Fatalf("second SeedOnce: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM scenarios`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	// Rows stay deleted — flag is irrevocable.
	if count != 0 {
		t.Errorf("want 0 rows (flag irrevocable), got %d", count)
	}
}
