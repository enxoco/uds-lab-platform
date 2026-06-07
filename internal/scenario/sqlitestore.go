package scenario

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SQLiteStore implements Store backed by a SQLite database.
type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

// DB returns the underlying *sql.DB, needed by tests that manipulate the DB directly.
func (s *SQLiteStore) DB() *sql.DB { return s.db }

// VersionSummary describes a scenario snapshot for history/rollback UI.
type VersionSummary struct {
	ID         int64     `json:"id"`
	ScenarioID string    `json:"scenario_id"`
	Comment    string    `json:"comment"`
	CreatedAt  time.Time `json:"created_at"`
}

// ScenarioRow is the raw DB row for admin listing (all statuses).
type ScenarioRow struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// Populated by joining scenario_files for scenario.yaml
	Title       string `json:"title"`
	Description string `json:"description"`
	Duration    int    `json:"duration"`
	Difficulty  string `json:"difficulty"`
	Playground  bool   `json:"playground"`
}

// --- Store interface implementation -----------------------------------------

func (s *SQLiteStore) List(ctx context.Context) ([]Summary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM scenarios WHERE status='published' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	// Collect IDs first so rows is closed before any secondary queries —
	// necessary because the single write connection can't be reacquired while
	// the outer Rows is still open.
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []Summary
	for _, id := range ids {
		sc, err := s.loadScenario(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, Summary{
			ID:          sc.ID,
			Title:       sc.Title,
			Description: sc.Description,
			Duration:    sc.Duration,
			Difficulty:  sc.Difficulty,
			Playground:  sc.Playground,
		})
	}
	return out, nil
}

func (s *SQLiteStore) Get(ctx context.Context, id string) (*Scenario, error) {
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT status FROM scenarios WHERE id=?`, id).Scan(&status)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("scenario %q not found", id)
	}
	if err != nil {
		return nil, err
	}
	return s.loadScenario(ctx, id)
}

func (s *SQLiteStore) GetVMData(ctx context.Context, id string) (*VMData, error) {
	files, err := s.allFiles(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load scenario %q: %w", id, err)
	}

	setupSh, ok := files["setup.sh"]
	if !ok {
		return nil, fmt.Errorf("scenario %q: setup.sh not found", id)
	}

	d := &VMData{
		SetupSh:       setupSh,
		VerifyScripts: map[string]string{},
	}

	for path, content := range files {
		if strings.HasPrefix(path, "verify/") {
			d.VerifyScripts[strings.TrimPrefix(path, "verify/")] = content
		}
	}

	if yamlContent, ok := files["scenario.yaml"]; ok {
		var meta struct {
			Browser    bool   `yaml:"browser"`
			Playground bool   `yaml:"playground"`
			Image      string `yaml:"image"`
			ServerType string `yaml:"serverType"`
		}
		if yaml.Unmarshal([]byte(yamlContent), &meta) == nil {
			d.Browser = meta.Browser
			d.Playground = meta.Playground
			d.Image = meta.Image
			d.ServerType = meta.ServerType
		}
	}

	return d, nil
}

// --- Admin methods ----------------------------------------------------------

// AdminList returns all scenarios regardless of status.
func (s *SQLiteStore) AdminList(ctx context.Context) ([]ScenarioRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, status, created_at, updated_at FROM scenarios ORDER BY id`)
	if err != nil {
		return nil, err
	}
	var out []ScenarioRow
	for rows.Next() {
		var r ScenarioRow
		if err := rows.Scan(&r.ID, &r.Status, &r.CreatedAt, &r.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Enrich each row with metadata from scenario.yaml — done after rows is
	// closed to avoid holding the single connection while making secondary queries.
	for i := range out {
		if yamlContent, err := s.fileContent(ctx, out[i].ID, "scenario.yaml"); err == nil {
			var meta struct {
				Title       string `yaml:"title"`
				Description string `yaml:"description"`
				Duration    int    `yaml:"duration"`
				Difficulty  string `yaml:"difficulty"`
				Playground  bool   `yaml:"playground"`
			}
			if yaml.Unmarshal([]byte(yamlContent), &meta) == nil {
				out[i].Title = meta.Title
				out[i].Description = meta.Description
				out[i].Duration = meta.Duration
				out[i].Difficulty = meta.Difficulty
				out[i].Playground = meta.Playground
			}
		}
	}
	return out, nil
}

// SetStatus transitions a scenario to draft, published, or archived.
// It also snapshots the current state as a new version when publishing.
func (s *SQLiteStore) SetStatus(ctx context.Context, id, status string) error {
	valid := map[string]bool{"draft": true, "published": true, "archived": true}
	if !valid[status] {
		return fmt.Errorf("invalid status %q", status)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var cur string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM scenarios WHERE id=?`, id).Scan(&cur); err != nil {
		return fmt.Errorf("scenario %q not found", id)
	}

	if status == "published" {
		if err := snapshotInTx(ctx, tx, id, "published"); err != nil {
			return fmt.Errorf("snapshot before publish: %w", err)
		}
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE scenarios SET status=?, updated_at=datetime('now') WHERE id=?`, status, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Versions returns the version history for a scenario, newest first.
func (s *SQLiteStore) Versions(ctx context.Context, id string) ([]VersionSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, scenario_id, comment, created_at
         FROM scenario_versions WHERE scenario_id=? ORDER BY id DESC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []VersionSummary
	for rows.Next() {
		var v VersionSummary
		if err := rows.Scan(&v.ID, &v.ScenarioID, &v.Comment, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Restore rolls back scenario files to a prior version snapshot.
func (s *SQLiteStore) Restore(ctx context.Context, scenarioID string, versionID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Validate that the requested version exists before touching live files.
	var versionExists int
	if err := tx.QueryRowContext(ctx,
		`SELECT count(*) FROM scenario_versions WHERE id=? AND scenario_id=?`,
		versionID, scenarioID).Scan(&versionExists); err != nil {
		return fmt.Errorf("check version: %w", err)
	}
	if versionExists == 0 {
		return fmt.Errorf("version %d not found for scenario %q", versionID, scenarioID)
	}

	// Snapshot current state before overwriting (so the rollback itself is reversible).
	if err := snapshotInTx(ctx, tx, scenarioID, "pre-restore"); err != nil {
		return fmt.Errorf("snapshot before restore: %w", err)
	}

	// Wipe current files and replace with version snapshot.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM scenario_files WHERE scenario_id=?`, scenarioID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
        INSERT INTO scenario_files(scenario_id, path, content)
        SELECT ?, path, content FROM scenario_version_files WHERE version_id=?`,
		scenarioID, versionID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE scenarios SET updated_at=datetime('now') WHERE id=?`, scenarioID); err != nil {
		return err
	}
	return tx.Commit()
}

// --- Internal helpers -------------------------------------------------------

func (s *SQLiteStore) loadScenario(ctx context.Context, id string) (*Scenario, error) {
	files, err := s.allFiles(ctx, id)
	if err != nil {
		return nil, err
	}

	yamlContent, ok := files["scenario.yaml"]
	if !ok {
		return nil, fmt.Errorf("scenario %q: scenario.yaml not found", id)
	}

	var sc Scenario
	if err := yaml.Unmarshal([]byte(yamlContent), &sc); err != nil {
		return nil, fmt.Errorf("parse scenario.yaml: %w", err)
	}
	sc.ID = id

	for i, step := range sc.Steps {
		content, ok := files[step.Text]
		if !ok {
			return nil, fmt.Errorf("step %d text file %q not found in DB", i+1, step.Text)
		}
		sc.Steps[i].Content = content
		sc.Steps[i].HasVerify = step.Verify != ""
	}

	return &sc, nil
}

func (s *SQLiteStore) allFiles(ctx context.Context, id string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path, content FROM scenario_files WHERE scenario_id=?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := map[string]string{}
	for rows.Next() {
		var path, content string
		if err := rows.Scan(&path, &content); err != nil {
			return nil, err
		}
		files[path] = content
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("scenario %q not found or has no files", id)
	}
	return files, rows.Err()
}

// --- File authoring methods -------------------------------------------------

func (s *SQLiteStore) CreateScenario(ctx context.Context, id string) error {
	scaffold := map[string]string{
		"scenario.yaml": "title: \"\"\ndescription: \"\"\nduration: 30\ndifficulty: beginner\nsteps:\n  - title: \"Step 1\"\n    text: steps/1-start.md\n    verify: verify/step1.sh\n",
		"setup.sh":               "#!/bin/bash\nset -euo pipefail\n\n# TODO: add setup steps\n\ntouch /var/log/lab-setup/ready\n",
		"steps/1-start.md":       "# Step 1\n\nDescribe the first step here.\n",
		"verify/step1.sh":        "#!/bin/bash\n# TODO: add verification logic\nexit 0\n",
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO scenarios(id, status) VALUES(?, 'draft')`, id,
	); err != nil {
		return fmt.Errorf("create scenario %q: %w", id, err)
	}
	for path, content := range scaffold {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO scenario_files(scenario_id, path, content) VALUES(?,?,?)`,
			id, path, content,
		); err != nil {
			return fmt.Errorf("scaffold %q: %w", path, err)
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) ListFiles(ctx context.Context, id string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path FROM scenario_files WHERE scenario_id=? ORDER BY path`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

func (s *SQLiteStore) GetFile(ctx context.Context, id, path string) (string, error) {
	content, err := s.fileContent(ctx, id, path)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("scenario %q: file %q not found", id, path)
	}
	return content, err
}

func (s *SQLiteStore) PutFile(ctx context.Context, id, path, content string) error {
	_, err := s.db.ExecContext(ctx, `
        INSERT INTO scenario_files(scenario_id, path, content) VALUES(?,?,?)
        ON CONFLICT(scenario_id, path) DO UPDATE SET content=excluded.content`,
		id, path, content)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE scenarios SET updated_at=datetime('now') WHERE id=?`, id)
	return err
}

// --- Internal helpers -------------------------------------------------------

func (s *SQLiteStore) fileContent(ctx context.Context, scenarioID, path string) (string, error) {
	var content string
	err := s.db.QueryRowContext(ctx,
		`SELECT content FROM scenario_files WHERE scenario_id=? AND path=?`,
		scenarioID, path).Scan(&content)
	return content, err
}

func snapshotInTx(ctx context.Context, tx *sql.Tx, scenarioID, comment string) error {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO scenario_versions(scenario_id, comment) VALUES(?,?)`, scenarioID, comment)
	if err != nil {
		return err
	}
	vID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
        INSERT INTO scenario_version_files(version_id, path, content)
        SELECT ?, path, content FROM scenario_files WHERE scenario_id=?`,
		vID, scenarioID)
	return err
}
