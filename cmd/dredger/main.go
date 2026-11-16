package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/alexzajac/the-dredger/internal/config"
	"github.com/alexzajac/the-dredger/internal/db"
	"github.com/alexzajac/the-dredger/internal/ingest"
	"github.com/alexzajac/the-dredger/internal/logging"
	"github.com/alexzajac/the-dredger/internal/ui"
	"github.com/alexzajac/the-dredger/internal/webexport"
)

const (
	commandSearch = "search"
	commandShow   = "show"
	commandTags   = "tags"
	commandStats  = "stats"
	commandImport = "import"
	commandAdd    = "add"
	commandClean  = "clean"
	commandReset  = "reset"
	commandWeb    = "web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	args, err := applyRootFlags(&cfg, os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Print(rootUsage)
			return
		}
		fmt.Fprintf(os.Stderr, "Error parsing arguments: %v\n", err)
		os.Exit(2)
	}

	if len(args) == 0 {
		runTUI(cfg)
		return
	}

	cmd, rest := args[0], args[1:]

	if cmd == "help" {
		fmt.Print(rootUsage)
		return
	}

	if !isReadOnlyCommand(cmd) && !isWriteCommand(cmd) {
		fmt.Fprintf(os.Stderr, "Unknown command %q.\n\n", cmd)
		fmt.Fprint(os.Stderr, rootUsage)
		os.Exit(2)
	}

	// Read-only commands open the database without a write lock and skip
	// InitSchema entirely. That is what lets an agent run `dredger search`
	// while the TUI holds the database open in another terminal.
	if isReadOnlyCommand(cmd) {
		// Parse command flags before opening the database, so help and usage
		// errors also work before the first interactive launch.

		switch cmd {
		case commandSearch:
			runSearch(cfg.DBPath, rest)
		case commandShow:
			runShow(cfg.DBPath, rest)
		case commandTags:
			runTags(cfg.DBPath, rest)
		case commandStats:
			database := openReadOnlyDatabase(cfg.DBPath)
			defer func() { _ = database.Close() }()
			runStats(database)
		}
		return
	}

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

	switch cmd {
	case commandImport:
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "Usage: dredger import <file|->")
			os.Exit(1)
		}
		runImport(database, rest[0])
	case commandAdd:
		runAdd(database, rest)
	case commandClean:
		runClean(database)
	case commandReset:
		runReset(database)
	case commandWeb:
		runWeb(database, rest)
	}
}

// readOnlyCommands never write, so they take the read-only database path.
var readOnlyCommands = map[string]bool{
	commandSearch: true,
	commandShow:   true,
	commandTags:   true,
	commandStats:  true,
}

func isReadOnlyCommand(cmd string) bool { return readOnlyCommands[cmd] }

func isWriteCommand(cmd string) bool {
	switch cmd {
	case commandImport, commandAdd, commandClean, commandReset, commandWeb:
		return true
	default:
		return false
	}
}

func openReadOnlyDatabase(path string) *sql.DB {
	database, err := db.OpenReadOnly(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening bookmark database at %s: %v — run dredger to create one.\n", path, err)
		os.Exit(1)
	}
	return database
}

// runTUI launches the interactive interface. It refuses to start without a
// terminal: Bubble Tea on a pipe blocks forever, which is exactly the failure an
// agent hits when a command is mistyped.
func runTUI(cfg config.Config) {
	info, err := os.Stdout.Stat()
	if err == nil && info.Mode()&os.ModeCharDevice == 0 {
		fmt.Fprint(os.Stderr, "dredger's interactive UI needs a terminal.\n\n")
		fmt.Fprint(os.Stderr, rootUsage)
		os.Exit(2)
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

	app := ui.NewApp(database, cfg, logger)
	p := tea.NewProgram(app)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}

func applyRootFlags(cfg *config.Config, args []string) ([]string, error) {
	fs := flag.NewFlagSet("dredger", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	service := fs.String("service", cfg.LLMService, "LLM service for dredging (ollama or lmstudio)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	cfg.LLMService = *service
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return fs.Args(), nil
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
	fs := flag.NewFlagSet(commandWeb, flag.ExitOnError)
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
	data, source, err := readImportInput(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", source, err)
		os.Exit(1)
	}

	importText(database, string(data), source)
}

func runAdd(database *sql.DB, args []string) {
	if len(args) > 0 {
		importText(database, strings.Join(args, " "), "arguments")
		return
	}

	info, err := os.Stdin.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking stdin: %v\n", err)
		os.Exit(1)
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		fmt.Fprintln(os.Stderr, "Usage: dredger add <url-or-text> [url-or-text...]")
		fmt.Fprintln(os.Stderr, "       pbpaste | dredger add")
		os.Exit(1)
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
		os.Exit(1)
	}

	importText(database, string(data), "stdin")
}

func readImportInput(path string) ([]byte, string, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		return data, "stdin", err
	}

	data, err := os.ReadFile(path)
	return data, "file", err
}

func importText(database *sql.DB, text, source string) {
	urls := ingest.ExtractURLs(text)
	if len(urls) == 0 {
		fmt.Printf("No URLs found in %s.\n", source)
		return
	}

	inserted, skipped, err := ingest.BulkInsert(database, urls)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error importing links: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Imported %d new links (%d duplicates skipped)\n", inserted, skipped)
}
