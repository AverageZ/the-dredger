package dredge

import (
	"strings"
	"testing"
)

func TestScrapeMetadata_TitleAndDescription(t *testing.T) {
	html := `<html><head>
		<title>My Page Title</title>
		<meta name="description" content="A description of the page.">
	</head><body></body></html>`

	meta := ScrapeMetadata(strings.NewReader(html))

	if meta.Title != "My Page Title" {
		t.Errorf("Title = %q, want %q", meta.Title, "My Page Title")
	}
	if meta.Description != "A description of the page." {
		t.Errorf("Description = %q, want %q", meta.Description, "A description of the page.")
	}
}

func TestScrapeMetadata_OGTags(t *testing.T) {
	html := `<html><head>
		<meta property="og:title" content="OG Title">
		<meta property="og:description" content="OG Description">
	</head><body></body></html>`

	meta := ScrapeMetadata(strings.NewReader(html))

	if meta.Title != "OG Title" {
		t.Errorf("Title = %q, want %q", meta.Title, "OG Title")
	}
	if meta.Description != "OG Description" {
		t.Errorf("Description = %q, want %q", meta.Description, "OG Description")
	}
}

func TestScrapeMetadata_TitleOverridesOG(t *testing.T) {
	html := `<html><head>
		<meta property="og:title" content="OG Title">
		<title>HTML Title</title>
	</head><body></body></html>`

	meta := ScrapeMetadata(strings.NewReader(html))

	if meta.Title != "HTML Title" {
		t.Errorf("Title = %q, want %q", meta.Title, "HTML Title")
	}
}

func TestScrapeMetadata_DescriptionPrecedence(t *testing.T) {
	html := `<html><head>
		<meta name="description" content="Meta Desc">
		<meta property="og:description" content="OG Desc">
	</head><body></body></html>`

	meta := ScrapeMetadata(strings.NewReader(html))

	// meta description comes first, OG only used as fallback
	if meta.Description != "Meta Desc" {
		t.Errorf("Description = %q, want %q", meta.Description, "Meta Desc")
	}
}

func TestScrapeMetadata_NoMeta(t *testing.T) {
	html := `<html><head></head><body><p>Hello</p></body></html>`

	meta := ScrapeMetadata(strings.NewReader(html))

	if meta.Title != "" {
		t.Errorf("Title = %q, want empty", meta.Title)
	}
	if meta.Description != "" {
		t.Errorf("Description = %q, want empty", meta.Description)
	}
}

func TestScrapeMetadata_EmptyBody(t *testing.T) {
	meta := ScrapeMetadata(strings.NewReader(""))

	if meta.Title != "" {
		t.Errorf("Title = %q, want empty", meta.Title)
	}
	if meta.Description != "" {
		t.Errorf("Description = %q, want empty", meta.Description)
	}
}
