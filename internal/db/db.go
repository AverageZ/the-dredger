package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// busyTimeoutMS is how long SQLite waits for a lock before giving up. Without
// it a second dredger process (an agent running `dredger search` while the TUI
// is open) fails immediately with SQLITE_BUSY instead of waiting out the writer.
const busyTimeoutMS = 5000

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
	if _, err := db.Exec(fmt.Sprintf("PRAGMA busy_timeout=%d", busyTimeoutMS)); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}

	return db, nil
}

// OpenReadOnly opens an existing database for reading only. Read paths use it so
// they never take a write lock and never create a database as a side effect of
// a typo'd path — an agent running `dredger search` should not be able to
// silently conjure an empty bookmark store.
//
// It deliberately does not create the directory, set WAL mode, or run any
// schema work; all of those are writes.
func OpenReadOnly(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("open db read-only: %w", err)
	}

	dsn := "file:" + path + "?mode=ro"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db read-only: %w", err)
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec(fmt.Sprintf("PRAGMA busy_timeout=%d", busyTimeoutMS)); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
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
