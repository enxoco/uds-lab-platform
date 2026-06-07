package scenario_test

import (
	"context"
	"database/sql"
	"testing"
	"testing/fstest"

	"github.com/enxoco/uds-lab-platform/internal/scenario"
	_ "modernc.org/sqlite"
)

// minimalFS returns a MapFS with one published-ready scenario.
// Path conventions match what both FSStore and SQLiteStore expect.
func minimalFS() fstest.MapFS {
	return fstest.MapFS{
		"alpha/scenario.yaml": {Data: []byte(`
title: "Alpha Scenario"
description: "Test scenario"
duration: 30
difficulty: beginner
browser: false
playground: false
steps:
  - title: "Step One"
    text: steps/1-first.md
    verify: verify/step1.sh
`)},
		"alpha/setup.sh":         {Data: []byte("#!/bin/bash\necho setup\n")},
		"alpha/steps/1-first.md": {Data: []byte("# First Step\nDo the thing.")},
		"alpha/verify/step1.sh":  {Data: []byte("#!/bin/bash\nexit 0\n")},
	}
}

// newMemDB opens an in-memory SQLite DB with the schema applied.
func newMemDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := scenario.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open mem db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// seededSQLiteStore returns an SQLiteStore seeded from minimalFS.
func seededSQLiteStore(t *testing.T) *scenario.SQLiteStore {
	t.Helper()
	db := newMemDB(t)
	ctx := context.Background()
	if err := scenario.SeedOnce(ctx, db, minimalFS()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return scenario.NewSQLiteStore(db)
}
