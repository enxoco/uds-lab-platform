package scenario_test

import (
	"context"
	"testing"

	"github.com/enxoco/uds-lab-platform/internal/scenario"
)

// --- SetStatus tests ---------------------------------------------------------

func TestSQLiteStore_SetStatus_DraftToPublished(t *testing.T) {
	db := newMemDB(t)
	ctx := context.Background()

	// Seed with status=published, then set back to draft first so we can
	// test the draft→published transition.
	if err := scenario.SeedOnce(ctx, db, minimalFS()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	store := scenario.NewSQLiteStore(db)

	// Move to draft first.
	if err := store.SetStatus(ctx, "alpha", "draft"); err != nil {
		t.Fatalf("SetStatus draft: %v", err)
	}

	// Now publish.
	if err := store.SetStatus(ctx, "alpha", "published"); err != nil {
		t.Fatalf("SetStatus published: %v", err)
	}

	// Confirm the row reflects the new status.
	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM scenarios WHERE id='alpha'`).Scan(&status); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "published" {
		t.Errorf("want status=published, got %q", status)
	}
}

func TestSQLiteStore_SetStatus_PublishCreatesVersionSnapshot(t *testing.T) {
	store := seededSQLiteStore(t)
	ctx := context.Background()

	// The scenario was seeded as published. Move to draft, then publish again
	// so SetStatus("published") runs once cleanly.
	if err := store.SetStatus(ctx, "alpha", "draft"); err != nil {
		t.Fatalf("SetStatus draft: %v", err)
	}
	if err := store.SetStatus(ctx, "alpha", "published"); err != nil {
		t.Fatalf("SetStatus published: %v", err)
	}

	versions, err := store.Versions(ctx, "alpha")
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(versions) != 1 {
		t.Errorf("want 1 version snapshot after publish, got %d", len(versions))
	}
}

func TestSQLiteStore_SetStatus_InvalidStatus_ReturnsError(t *testing.T) {
	store := seededSQLiteStore(t)
	ctx := context.Background()

	err := store.SetStatus(ctx, "alpha", "bogus")
	if err == nil {
		t.Fatal("expected error for invalid status, got nil")
	}
}

// --- Restore tests -----------------------------------------------------------

func TestSQLiteStore_Restore_ReplacesFilesWithSnapshotContent(t *testing.T) {
	store := seededSQLiteStore(t)
	ctx := context.Background()

	// Create a version snapshot by publishing (seeded as published, so draft first).
	if err := store.SetStatus(ctx, "alpha", "draft"); err != nil {
		t.Fatalf("SetStatus draft: %v", err)
	}
	if err := store.SetStatus(ctx, "alpha", "published"); err != nil {
		t.Fatalf("SetStatus published: %v", err)
	}

	// Capture current version ID.
	versions, err := store.Versions(ctx, "alpha")
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(versions) == 0 {
		t.Fatal("need at least one version before restore")
	}
	targetVersion := versions[0].ID

	// Corrupt setup.sh in the live files.
	if _, err := store.DB().ExecContext(ctx,
		`UPDATE scenario_files SET content='corrupted' WHERE scenario_id='alpha' AND path='setup.sh'`,
	); err != nil {
		t.Fatalf("corrupt setup.sh: %v", err)
	}

	// Restore to the snapshot.
	if err := store.Restore(ctx, "alpha", targetVersion); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Verify the file is back to the original content.
	vmd, err := store.GetVMData(ctx, "alpha")
	if err != nil {
		t.Fatalf("GetVMData after restore: %v", err)
	}
	want := "#!/bin/bash\necho setup\n"
	if vmd.SetupSh != want {
		t.Errorf("after restore: SetupSh=%q, want %q", vmd.SetupSh, want)
	}
}

func TestSQLiteStore_Restore_CreatesPreRestoreSnapshot(t *testing.T) {
	store := seededSQLiteStore(t)
	ctx := context.Background()

	// Create first snapshot via publish cycle.
	if err := store.SetStatus(ctx, "alpha", "draft"); err != nil {
		t.Fatalf("draft: %v", err)
	}
	if err := store.SetStatus(ctx, "alpha", "published"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	versionsBefore, err := store.Versions(ctx, "alpha")
	if err != nil {
		t.Fatalf("Versions before: %v", err)
	}
	targetVersion := versionsBefore[0].ID

	// Restore — this should add a pre-restore snapshot.
	if err := store.Restore(ctx, "alpha", targetVersion); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	versionsAfter, err := store.Versions(ctx, "alpha")
	if err != nil {
		t.Fatalf("Versions after: %v", err)
	}
	if len(versionsAfter) != len(versionsBefore)+1 {
		t.Errorf("want %d versions after restore, got %d", len(versionsBefore)+1, len(versionsAfter))
	}
}

func TestSQLiteStore_Restore_NonexistentVersionID_ReturnsError(t *testing.T) {
	store := seededSQLiteStore(t)
	ctx := context.Background()

	// Capture current setup.sh to verify it's unchanged after failed restore.
	vmdBefore, err := store.GetVMData(ctx, "alpha")
	if err != nil {
		t.Fatalf("GetVMData before: %v", err)
	}

	err = store.Restore(ctx, "alpha", 999999)
	if err == nil {
		t.Fatal("expected error for non-existent versionID, got nil")
	}

	// Files must be unchanged.
	vmdAfter, err := store.GetVMData(ctx, "alpha")
	if err != nil {
		t.Fatalf("GetVMData after failed restore: %v", err)
	}
	if vmdAfter.SetupSh != vmdBefore.SetupSh {
		t.Errorf("files changed after failed restore: before=%q after=%q", vmdBefore.SetupSh, vmdAfter.SetupSh)
	}
}

// --- AdminList tests ---------------------------------------------------------

func TestSQLiteStore_AdminList_ReturnsAllStatuses(t *testing.T) {
	store := seededSQLiteStore(t)
	ctx := context.Background()

	// Add a draft scenario directly in the DB.
	if _, err := store.DB().ExecContext(ctx,
		`INSERT INTO scenarios(id, status) VALUES('beta','draft')`,
	); err != nil {
		t.Fatalf("insert beta: %v", err)
	}

	rows, err := store.AdminList(ctx)
	if err != nil {
		t.Fatalf("AdminList: %v", err)
	}

	statusByID := make(map[string]string)
	for _, r := range rows {
		statusByID[r.ID] = r.Status
	}

	if statusByID["alpha"] != "published" {
		t.Errorf("alpha: want published, got %q", statusByID["alpha"])
	}
	if statusByID["beta"] != "draft" {
		t.Errorf("beta: want draft, got %q", statusByID["beta"])
	}
}

// --- Versions tests ----------------------------------------------------------

func TestSQLiteStore_Versions_NewestFirst(t *testing.T) {
	store := seededSQLiteStore(t)
	ctx := context.Background()

	// Create two version snapshots by going draft→publish twice.
	if err := store.SetStatus(ctx, "alpha", "draft"); err != nil {
		t.Fatalf("draft 1: %v", err)
	}
	if err := store.SetStatus(ctx, "alpha", "published"); err != nil {
		t.Fatalf("publish 1: %v", err)
	}
	if err := store.SetStatus(ctx, "alpha", "draft"); err != nil {
		t.Fatalf("draft 2: %v", err)
	}
	if err := store.SetStatus(ctx, "alpha", "published"); err != nil {
		t.Fatalf("publish 2: %v", err)
	}

	versions, err := store.Versions(ctx, "alpha")
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(versions) < 2 {
		t.Fatalf("want at least 2 versions, got %d", len(versions))
	}

	// IDs are auto-increment; newest = highest ID should appear first.
	if versions[0].ID <= versions[1].ID {
		t.Errorf("versions not newest-first: versions[0].ID=%d versions[1].ID=%d",
			versions[0].ID, versions[1].ID)
	}
}
