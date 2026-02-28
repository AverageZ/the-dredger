package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/alexzajac/the-dredger/internal/db"
	"github.com/alexzajac/the-dredger/internal/ingest"
	"github.com/alexzajac/the-dredger/internal/ui"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding home directory: %v\n", err)
		os.Exit(1)
	}

	dbPath := filepath.Join(home, ".dredger", "dredger.db")

	database, err := db.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := db.InitSchema(database); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing schema: %v\n", err)
		os.Exit(1)
	}

	if len(os.Args) >= 3 && os.Args[1] == "import" {
		runImport(database, os.Args[2])
		return
	}

	app := ui.NewApp(database)
	p := tea.NewProgram(app)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}

func runImport(database *sql.DB, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	urls := ingest.ExtractURLs(string(data))
	if len(urls) == 0 {
		fmt.Println("No URLs found in file.")
		return
	}

	inserted, skipped, err := ingest.BulkInsert(database, urls)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error importing links: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Imported %d new links (%d duplicates skipped)\n", inserted, skipped)
}
