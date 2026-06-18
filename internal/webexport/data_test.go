package webexport

import (
	"testing"
	"time"

	"github.com/alexzajac/the-dredger/internal/db"
	"github.com/alexzajac/the-dredger/internal/model"
)

func TestDomainOf(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"simple", "https://github.com/foo", "github.com"},
		{"strips www", "https://www.example.com/path", "example.com"},
		{"strips port", "http://localhost:8080/x", "localhost"},
		{"uppercase host", "https://GitHub.COM/a", "github.com"},
		{"www and port", "https://www.example.com:443/", "example.com"},
		{"relative path", "/just/a/path", ""},
		{"mailto", "mailto:someone@example.com", ""},
		{"empty", "", ""},
		{"whitespace trimmed", "  https://news.ycombinator.com/  ", "news.ycombinator.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domainOf(tt.url); got != tt.want {
				t.Errorf("domainOf(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestAggregateDomains(t *testing.T) {
	links := []model.Link{
		{URL: "https://github.com/a"},
		{URL: "https://www.github.com/b"},
		{URL: "https://example.com/c"},
		{URL: "/relative"},      // no host, skipped
		{URL: "mailto:x@y.com"}, // no host, skipped
		{URL: "https://github.com/d"},
	}
	got := aggregateDomains(links)

	if len(got) != 2 {
		t.Fatalf("expected 2 domains, got %d: %+v", len(got), got)
	}
	// Sorted by count desc: github.com (3) before example.com (1).
	if got[0].Label != "github.com" || got[0].Count != 3 {
		t.Errorf("got[0] = %+v, want {github.com 3}", got[0])
	}
	if got[1].Label != "example.com" || got[1].Count != 1 {
		t.Errorf("got[1] = %+v, want {example.com 1}", got[1])
	}
}

func TestBuildExport(t *testing.T) {
	links := []model.Link{
		{ID: 1, URL: "https://a.com", Status: model.Unprocessed, DredgeState: model.DredgeComplete},
		{ID: 2, URL: "https://b.com", Status: model.Saved, DredgeState: model.DredgeCapsized, Tags: []string{"go"}},
		{ID: 3, URL: "https://c.com", Status: model.Saved, DredgeState: model.DredgeCrawling},
		{ID: 4, URL: "https://d.com", Status: model.Saved, DredgeState: model.DredgeNone},
	}
	stats := db.LinkStats{Unprocessed: 1, Saved: 3, Pruned: 0, Total: 4}
	tags := []db.TagCount{{Tag: "go", Count: 1}}

	exp := buildExport(links, stats, tags)

	if exp.Version != schemaVersion {
		t.Errorf("Version = %d, want %d", exp.Version, schemaVersion)
	}
	if exp.GeneratedAt == "" {
		t.Error("GeneratedAt is empty")
	}
	if _, err := time.Parse(time.RFC3339, exp.GeneratedAt); err != nil {
		t.Errorf("GeneratedAt not RFC3339: %v", err)
	}

	if exp.Stats.Bookmarks != 4 || exp.Stats.Inbox != 1 || exp.Stats.Saved != 3 || exp.Stats.Total != 4 {
		t.Errorf("Stats = %+v, want bookmarks=4 inbox=1 saved=3 total=4", exp.Stats)
	}

	// Enrichment health bucketing: 1 complete, 1 capsized, 2 pending (crawling + none).
	wantHealth := EnrichHealth{Complete: 1, Capsized: 1, Pending: 2}
	if exp.EnrichmentHealth != wantHealth {
		t.Errorf("EnrichmentHealth = %+v, want %+v", exp.EnrichmentHealth, wantHealth)
	}

	if len(exp.Links) != 4 {
		t.Fatalf("expected 4 link DTOs, got %d", len(exp.Links))
	}
	// Enum mapping and status vocabulary.
	if exp.Links[0].Status != "bookmark" {
		t.Errorf("link[0].Status = %q, want bookmark", exp.Links[0].Status)
	}
	if exp.Links[1].Status != "bookmark" || exp.Links[1].DredgeState != "Capsized" {
		t.Errorf("link[1] = %+v, want bookmark/Capsized", exp.Links[1])
	}
	// Tags must never be nil for the browser.
	for i, l := range exp.Links {
		if l.Tags == nil {
			t.Errorf("link[%d].Tags is nil", i)
		}
	}

	if len(exp.Tags) != 1 || exp.Tags[0].Label != "go" || exp.Tags[0].Count != 1 {
		t.Errorf("Tags = %+v, want [{go 1}]", exp.Tags)
	}
}
