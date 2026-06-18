package webexport

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alexzajac/the-dredger/internal/db"
	"github.com/alexzajac/the-dredger/internal/model"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.InitSchema(database); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

// insert adds a link with the given status and returns its id.
func insert(t *testing.T, database *sql.DB, url string, status model.Status, tags []string) int64 {
	t.Helper()
	id, err := db.InsertLink(database, model.Link{URL: url, Title: url, Tags: tags, Status: status})
	if err != nil {
		t.Fatalf("insert link: %v", err)
	}
	return id
}

func TestRunWritesStaticSite(t *testing.T) {
	database := setupTestDB(t)
	insert(t, database, "https://github.com/a", model.Saved, []string{"go"})
	insert(t, database, "https://example.com/b", model.Unprocessed, nil)
	insert(t, database, "https://gone.com/c", model.Pruned, nil)

	outDir := filepath.Join(t.TempDir(), "site")
	if err := Run(database, Options{OutDir: outDir}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, name := range []string{"index.html", "app.css", "app.js", "data.json", "README.txt"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}

	// index.html must carry the inline JSON blob and parse as our Export.
	html, err := os.ReadFile(filepath.Join(outDir, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	blob := extractDataBlob(t, string(html))
	var exp Export
	if err := json.Unmarshal([]byte(blob), &exp); err != nil {
		t.Fatalf("inline blob is not valid Export JSON: %v", err)
	}

	// Default run excludes pruned: 1 saved + 1 inbox = 2 links.
	if len(exp.Links) != 2 {
		t.Errorf("expected 2 links (pruned excluded), got %d", len(exp.Links))
	}
	// Stats reflect the full DB regardless of what is exported.
	if exp.Stats.Pruned != 1 {
		t.Errorf("Stats.Pruned = %d, want 1", exp.Stats.Pruned)
	}
}

func TestRunIncludePruned(t *testing.T) {
	database := setupTestDB(t)
	insert(t, database, "https://github.com/a", model.Saved, nil)
	insert(t, database, "https://gone.com/c", model.Pruned, nil)

	outDir := filepath.Join(t.TempDir(), "site")
	if err := Run(database, Options{OutDir: outDir, IncludePruned: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "data.json"))
	if err != nil {
		t.Fatalf("read data.json: %v", err)
	}
	var exp Export
	if err := json.Unmarshal(data, &exp); err != nil {
		t.Fatalf("data.json invalid: %v", err)
	}
	if len(exp.Links) != 2 {
		t.Errorf("expected 2 links with pruned included, got %d", len(exp.Links))
	}
}

func TestRunDefaultsOutDir(t *testing.T) {
	database := setupTestDB(t)
	insert(t, database, "https://github.com/a", model.Saved, nil)

	// Run from a temp working dir so the default ./dredger-web lands there.
	wd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	if err := Run(database, Options{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "dredger-web", "index.html")); err != nil {
		t.Errorf("expected default out dir dredger-web/index.html: %v", err)
	}
}

// extractDataBlob pulls the contents of the inline <script id="dredger-data">.
func extractDataBlob(t *testing.T, html string) string {
	t.Helper()
	const open = `<script id="dredger-data" type="application/json">`
	i := strings.Index(html, open)
	if i < 0 {
		t.Fatal("dredger-data script tag not found in index.html")
	}
	rest := html[i+len(open):]
	j := strings.Index(rest, "</script>")
	if j < 0 {
		t.Fatal("unterminated dredger-data script tag")
	}
	return rest[:j]
}
