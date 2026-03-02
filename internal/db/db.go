package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	return db, nil
}

func InitSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS links (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			url         TEXT NOT NULL UNIQUE,
			title       TEXT DEFAULT '',
			description TEXT DEFAULT '',
			tags        TEXT DEFAULT '',
			status      INTEGER DEFAULT 0,
			date_added  DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		return fmt.Errorf("create links table: %w", err)
	}

	// Migrations: add columns if they don't exist
	migrations := []string{
		`ALTER TABLE links ADD COLUMN enriched INTEGER DEFAULT 0`,
		`ALTER TABLE links ADD COLUMN dredge_state INTEGER DEFAULT 0`,
		`ALTER TABLE links ADD COLUMN dredge_error TEXT DEFAULT ''`,
		`ALTER TABLE links ADD COLUMN summary TEXT DEFAULT ''`,
	}
	for _, m := range migrations {
		_, err = db.Exec(m)
		if err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return fmt.Errorf("migration %q: %w", m, err)
		}
	}

	// Indexes for performance
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_links_status ON links(status)`,
		`CREATE INDEX IF NOT EXISTS idx_links_enriched ON links(enriched)`,
		`CREATE INDEX IF NOT EXISTS idx_links_dredge_state ON links(dredge_state)`,
	}
	for _, idx := range indexes {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}

	return nil
}
