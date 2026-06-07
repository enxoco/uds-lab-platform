package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"testing/fstest"

	"github.com/enxoco/uds-lab-platform/internal/scenario"
	_ "modernc.org/sqlite"
)

// minimalStaticFS satisfies the static file server without real assets.
var minimalStaticFS = fstest.MapFS{
	"index.html": {Data: []byte("<html></html>")},
}

// newTestServer builds a server wired to a seeded in-memory SQLiteStore.
// authEnabled is false so requireAdmin passes through without cookie checks.
func newTestServer(t *testing.T) *server {
	t.Helper()
	db, err := scenario.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open mem db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	seedFS := fstest.MapFS{
		"alpha/scenario.yaml": {Data: []byte(`
title: "Alpha"
description: "Test"
duration: 30
difficulty: beginner
steps:
  - title: "Step 1"
    text: steps/1.md
    verify: verify/step1.sh
`)},
		"alpha/setup.sh":        {Data: []byte("#!/bin/bash\necho setup\n")},
		"alpha/steps/1.md":      {Data: []byte("# Step\nDo the thing.")},
		"alpha/verify/step1.sh": {Data: []byte("#!/bin/bash\nexit 0\n")},
	}
	if err := scenario.SeedOnce(context.Background(), db, seedFS); err != nil {
		t.Fatalf("seed: %v", err)
	}
	sqlStore := scenario.NewSQLiteStore(db)
	return &server{
		scenarios:      sqlStore,
		adminScenarios: sqlStore,
		staticFS:       minimalStaticFS,
		authEnabled:    false,
		adminUsers:     map[string]bool{},
	}
}

func TestAdminListScenarios_NilAdminStore_Returns501(t *testing.T) {
	srv := &server{
		scenarios:   scenario.NewFSStore(fstest.MapFS{}),
		staticFS:    minimalStaticFS,
		authEnabled: false,
		adminUsers:  map[string]bool{},
		// adminScenarios intentionally nil
	}
	h := srv.routes()
	r := httptest.NewRequest(http.MethodGet, "/api/admin/scenarios", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("want 501, got %d", w.Code)
	}
}

func TestAdminListScenarios_ReturnsAll(t *testing.T) {
	srv := newTestServer(t)
	h := srv.routes()

	r := httptest.NewRequest(http.MethodGet, "/api/admin/scenarios", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var rows []scenario.ScenarioRow
	if err := json.NewDecoder(w.Body).Decode(&rows); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 scenario, got %d", len(rows))
	}
	if rows[0].ID != "alpha" {
		t.Errorf("want ID=alpha, got %q", rows[0].ID)
	}
}

func TestAdminGetScenario_ReturnsScenario(t *testing.T) {
	srv := newTestServer(t)
	h := srv.routes()

	r := httptest.NewRequest(http.MethodGet, "/api/admin/scenarios/alpha", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var sc scenario.Scenario
	if err := json.NewDecoder(w.Body).Decode(&sc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if sc.ID != "alpha" {
		t.Errorf("want ID=alpha, got %q", sc.ID)
	}
}

func TestAdminGetScenario_Unknown_Returns404(t *testing.T) {
	srv := newTestServer(t)
	h := srv.routes()

	r := httptest.NewRequest(http.MethodGet, "/api/admin/scenarios/no-such-thing", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestAdminPublishScenario_TransitionsToPublished(t *testing.T) {
	srv := newTestServer(t)
	h := srv.routes()

	// Seed lands as published; unpublish first so we can test publish.
	unpub := httptest.NewRequest(http.MethodPost, "/api/admin/scenarios/alpha/unpublish", nil)
	srv.routes().ServeHTTP(httptest.NewRecorder(), unpub)

	r := httptest.NewRequest(http.MethodPost, "/api/admin/scenarios/alpha/publish", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}

	// Confirm status via list.
	list := httptest.NewRequest(http.MethodGet, "/api/admin/scenarios", nil)
	lw := httptest.NewRecorder()
	h.ServeHTTP(lw, list)
	var rows []scenario.ScenarioRow
	_ = json.NewDecoder(lw.Body).Decode(&rows)
	if len(rows) == 0 || rows[0].Status != "published" {
		t.Errorf("want status=published, got %q", func() string {
			if len(rows) > 0 {
				return rows[0].Status
			}
			return "(empty)"
		}())
	}
}

func TestAdminUnpublishScenario_TransitionsToDraft(t *testing.T) {
	srv := newTestServer(t)
	h := srv.routes()

	r := httptest.NewRequest(http.MethodPost, "/api/admin/scenarios/alpha/unpublish", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}

	list := httptest.NewRequest(http.MethodGet, "/api/admin/scenarios", nil)
	lw := httptest.NewRecorder()
	h.ServeHTTP(lw, list)
	var rows []scenario.ScenarioRow
	_ = json.NewDecoder(lw.Body).Decode(&rows)
	if len(rows) == 0 || rows[0].Status != "draft" {
		t.Errorf("want status=draft, got rows=%v", rows)
	}
}

func TestAdminListVersions_AfterPublish_HasEntry(t *testing.T) {
	srv := newTestServer(t)
	h := srv.routes()

	// Publish creates a snapshot version.
	pub := httptest.NewRequest(http.MethodPost, "/api/admin/scenarios/alpha/publish", nil)
	srv.routes().ServeHTTP(httptest.NewRecorder(), pub)

	r := httptest.NewRequest(http.MethodGet, "/api/admin/scenarios/alpha/versions", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	var versions []scenario.VersionSummary
	if err := json.NewDecoder(w.Body).Decode(&versions); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(versions) == 0 {
		t.Fatal("expected at least 1 version after publish, got 0")
	}
}

func TestAdminRestoreVersion_Returns200(t *testing.T) {
	srv := newTestServer(t)
	h := srv.routes()

	// Publish to create a snapshot, then get its ID.
	pub := httptest.NewRequest(http.MethodPost, "/api/admin/scenarios/alpha/publish", nil)
	h.ServeHTTP(httptest.NewRecorder(), pub)

	vr := httptest.NewRequest(http.MethodGet, "/api/admin/scenarios/alpha/versions", nil)
	vw := httptest.NewRecorder()
	h.ServeHTTP(vw, vr)
	var versions []scenario.VersionSummary
	if err := json.NewDecoder(vw.Body).Decode(&versions); err != nil || len(versions) == 0 {
		t.Fatalf("need at least one version to restore; got: %v %v", versions, err)
	}

	url := "/api/admin/scenarios/alpha/versions/" + strconv.FormatInt(versions[0].ID, 10) + "/restore"
	r := httptest.NewRequest(http.MethodPost, url, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}
