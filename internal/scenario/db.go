package scenario

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS system_kv (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS scenarios (
    id          TEXT PRIMARY KEY,
    status      TEXT NOT NULL DEFAULT 'draft'
                    CHECK(status IN ('draft','published','archived')),
    created_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS scenario_files (
    scenario_id TEXT NOT NULL REFERENCES scenarios(id) ON DELETE CASCADE,
    path        TEXT NOT NULL,
    content     TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (scenario_id, path)
);

CREATE TABLE IF NOT EXISTS scenario_versions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    scenario_id TEXT NOT NULL REFERENCES scenarios(id) ON DELETE CASCADE,
    comment     TEXT NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS scenario_version_files (
    version_id  INTEGER NOT NULL REFERENCES scenario_versions(id) ON DELETE CASCADE,
    path        TEXT NOT NULL,
    content     TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (version_id, path)
);

CREATE INDEX IF NOT EXISTS idx_scenario_versions_scenario
    ON scenario_versions(scenario_id, created_at DESC);
`

// OpenDB opens (or creates) the SQLite database, applies the schema, and
// returns a single-connection db handle configured for WAL mode.
func OpenDB(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_journal=WAL&_timeout=5000&_foreign_keys=on", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	// Single writer — SQLite WAL supports concurrent readers but serialises writes.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return db, nil
}

// SeedOnce seeds the database from an fs.FS the first time it is opened.
// The seed is irrevocable: once the system_kv row is written, re-seeding
// never happens regardless of row count.
func SeedOnce(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	var v string
	err := db.QueryRowContext(ctx, `SELECT value FROM system_kv WHERE key='seeded'`).Scan(&v)
	if err == nil {
		return nil // already seeded
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("check seed flag: %w", err)
	}

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return fmt.Errorf("read embedded scenarios: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		slug := e.Name()
		if err := seedScenario(ctx, tx, fsys, slug); err != nil {
			return fmt.Errorf("seed scenario %q: %w", slug, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO system_kv(key,value) VALUES('seeded','1')`); err != nil {
		return fmt.Errorf("write seed flag: %w", err)
	}
	return tx.Commit()
}

func seedScenario(ctx context.Context, tx *sql.Tx, fsys fs.FS, slug string) error {
	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO scenarios(id, status) VALUES(?, 'published')`, slug,
	); err != nil {
		return err
	}

	// Walk all files under the scenario directory and insert them.
	return fs.WalkDir(fsys, slug, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := path[len(slug)+1:] // strip "slug/" prefix
		content, readErr := fs.ReadFile(fsys, path)
		if readErr != nil {
			return readErr
		}
		_, execErr := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO scenario_files(scenario_id, path, content) VALUES(?,?,?)`,
			slug, rel, string(content),
		)
		return execErr
	})
}
