package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/alexzajac/the-dredger/internal/config"
	"github.com/alexzajac/the-dredger/internal/db"
	"github.com/alexzajac/the-dredger/internal/ingest"
	"github.com/alexzajac/the-dredger/internal/logging"
	"github.com/alexzajac/the-dredger/internal/ui"
	"github.com/alexzajac/the-dredger/internal/webexport"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	logger, closeLog, err := logging.Setup(cfg.LogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error setting up logging: %v\n", err)
		os.Exit(1)
	}
	defer closeLog()

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = database.Close() }()

	if err := db.InitSchema(database); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing schema: %v\n", err)
		os.Exit(1)
	}

	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "import":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "Usage: dredger import <file>")
				os.Exit(1)
			}
			runImport(database, os.Args[2])
			return
		case "stats":
			runStats(database)
			return
		case "clean":
			runClean(database)
			return
		case "reset":
			runReset(database)
			return
		case "web":
			runWeb(database, os.Args[2:])
			return
		}
	}

	app := ui.NewApp(database, cfg, logger)
	p := tea.NewProgram(app)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}

func runStats(database *sql.DB) {
	stats, err := db.CountLinksByStatus(database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting stats: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Bookmarks: %d\nDeleted:   %d\nTotal:     %d\n",
		stats.Unprocessed+stats.Saved, stats.Pruned, stats.Total)
}

func runClean(database *sql.DB) {
	removed, err := db.DeletePrunedLinks(database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error cleaning pruned links: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Removed %d pruned links.\n", removed)
}

func runReset(database *sql.DB) {
	fmt.Print("This will delete ALL links. Are you sure? [y/N] ")
	var answer string
	_, _ = fmt.Scanln(&answer)
	if answer != "y" && answer != "Y" {
		fmt.Println("Aborted.")
		return
	}
	removed, err := db.DeleteAllLinks(database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resetting database: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Deleted %d links. Database is now empty.\n", removed)
}

func runWeb(database *sql.DB, args []string) {
	fs := flag.NewFlagSet("web", flag.ExitOnError)
	out := fs.String("out", webexport.DefaultOutDir, "output folder for the static export")
	includePruned := fs.Bool("include-pruned", false, "include legacy pruned links in the export")
	_ = fs.Parse(args)

	opts := webexport.Options{OutDir: *out, IncludePruned: *includePruned}
	if err := webexport.Run(database, opts); err != nil {
		fmt.Fprintf(os.Stderr, "Error exporting web dashboard: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Exported static dashboard to %s\nOpen %s/index.html in a browser.\n", opts.OutDir, opts.OutDir)
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
