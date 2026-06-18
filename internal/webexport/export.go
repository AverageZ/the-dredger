// Package webexport renders a read-only, self-contained static "idea mine"
// dashboard from the Dredger database. It writes a folder (index.html, app.css,
// app.js, data.json) the user can open directly or publish anywhere; no server
// is involved.
package webexport

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alexzajac/the-dredger/internal/db"
	"github.com/alexzajac/the-dredger/internal/model"
)

// DefaultOutDir is used when Options.OutDir is empty.
const DefaultOutDir = "./dredger-web"

const readmeText = `The Dredger — static export

This folder is a read-only snapshot of your Dredger database. Open index.html in
any browser (double-clicking works — no server needed).

It is a LOCAL snapshot and may contain private URLs, titles, and AI summaries.
Be deliberate about where you publish or share it.

Files:
  index.html  the dashboard (data is inlined, so it works over file://)
  app.css     styles
  app.js      dashboard logic
  data.json   the same snapshot as standalone JSON, for tooling/diffing
`

// Options controls a single export run.
type Options struct {
	OutDir        string // destination folder; created if absent
	IncludePruned bool   // include pruned links in the export
}

// Run reads the database, builds the snapshot, and writes the static site to
// opts.OutDir. The output folder is created if needed; the known output files
// are overwritten. Nothing is ever recursively deleted.
func Run(database *sql.DB, opts Options) error {
	outDir := opts.OutDir
	if outDir == "" {
		outDir = DefaultOutDir
	}

	links, err := db.GetLinks(database)
	if err != nil {
		return fmt.Errorf("load links: %w", err)
	}
	if !opts.IncludePruned {
		links = dropPruned(links)
	}

	stats, err := db.CountLinksByStatus(database)
	if err != nil {
		return fmt.Errorf("count links: %w", err)
	}

	tags, err := db.GetTagCounts(database)
	if err != nil {
		return fmt.Errorf("tag counts: %w", err)
	}

	export := buildExport(links, stats, tags)
	blob, err := json.Marshal(export)
	if err != nil {
		return fmt.Errorf("marshal export: %w", err)
	}
	prettyBlob, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal export (indent): %w", err)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}

	if err := renderIndex(outDir, blob, export.GeneratedAt); err != nil {
		return err
	}

	files := map[string][]byte{
		"app.css":    appCSS,
		"app.js":     appJS,
		"data.json":  prettyBlob,
		"README.txt": []byte(readmeText),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(outDir, name), content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}

	return nil
}

// renderIndex writes index.html with the snapshot inlined as a <script> blob.
func renderIndex(outDir string, blob []byte, generatedAt string) error {
	f, err := os.Create(filepath.Join(outDir, "index.html"))
	if err != nil {
		return fmt.Errorf("create index.html: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := indexTmpl.Execute(f, pageData{Data: string(blob), GeneratedAt: generatedAt}); err != nil {
		return fmt.Errorf("render index.html: %w", err)
	}
	return nil
}

// dropPruned returns links excluding any with Pruned status, preserving order.
func dropPruned(links []model.Link) []model.Link {
	out := links[:0:0]
	for _, l := range links {
		if l.Status != model.Pruned {
			out = append(out, l)
		}
	}
	return out
}
