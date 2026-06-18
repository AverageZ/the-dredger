package webexport

import (
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/alexzajac/the-dredger/internal/db"
	"github.com/alexzajac/the-dredger/internal/model"
)

// Export is the top-level JSON contract consumed by the static dashboard. It is
// versioned so the embedded app.js can guard against an incompatible snapshot.
type Export struct {
	GeneratedAt      string       `json:"generatedAt"` // RFC3339
	Version          int          `json:"version"`
	Stats            Stats        `json:"stats"`
	EnrichmentHealth EnrichHealth `json:"enrichmentHealth"`
	Links            []LinkDTO    `json:"links"`
	Tags             []Count      `json:"tags"`    // bookmark-only, sorted by count desc
	Domains          []Count      `json:"domains"` // all exported links, sorted by count desc
}

// schemaVersion is the current Export.Version. Bump when the JSON contract changes.
const schemaVersion = 2

// Stats mirrors db.LinkStats but adds a bookmark total for the collapsed
// non-deleted view. The old inbox/saved keys remain for compatibility.
type Stats struct {
	Bookmarks int `json:"bookmarks"`
	Inbox     int `json:"inbox"`
	Saved     int `json:"saved"`
	Pruned    int `json:"pruned"`
	Total     int `json:"total"`
}

// EnrichHealth buckets every exported link by its dredge state.
type EnrichHealth struct {
	Complete int `json:"complete"` // DredgeComplete
	Capsized int `json:"capsized"` // DredgeCapsized (failed)
	Pending  int `json:"pending"`  // DredgeNone / Crawling / Crunching
}

// LinkDTO is the wire representation of a link. It deliberately omits internal
// fields (raw enums, DredgeError) and precomputes Domain so the browser never
// has to parse URLs.
type LinkDTO struct {
	ID          int64    `json:"id"`
	URL         string   `json:"url"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	Tags        []string `json:"tags"` // never null
	Domain      string   `json:"domain"`
	Status      string   `json:"status"`      // "bookmark" | "deleted"
	DredgeState string   `json:"dredgeState"` // "Complete" | "Capsized" | "" ...
	DateAdded   string   `json:"dateAdded"`   // RFC3339
}

// Count is a generic label/count pair used for tag and domain rankings.
type Count struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// buildExport assembles the JSON contract from the loaded links and the
// pre-aggregated stats and tag counts. Tags from the DB layer are bookmark-only;
// domains are aggregated here across every exported link.
func buildExport(links []model.Link, stats db.LinkStats, tags []db.TagCount) Export {
	dtos := make([]LinkDTO, 0, len(links))
	var health EnrichHealth
	for _, l := range links {
		dtos = append(dtos, toDTO(l))
		switch l.DredgeState {
		case model.DredgeComplete:
			health.Complete++
		case model.DredgeCapsized:
			health.Capsized++
		default:
			health.Pending++
		}
	}

	tagCounts := make([]Count, 0, len(tags))
	for _, t := range tags {
		tagCounts = append(tagCounts, Count{Label: t.Tag, Count: t.Count})
	}

	return Export{
		GeneratedAt: time.Now().Format(time.RFC3339),
		Version:     schemaVersion,
		Stats: Stats{
			Bookmarks: stats.Unprocessed + stats.Saved,
			Inbox:     stats.Unprocessed,
			Saved:     stats.Saved,
			Pruned:    stats.Pruned,
			Total:     stats.Total,
		},
		EnrichmentHealth: health,
		Links:            dtos,
		Tags:             tagCounts,
		Domains:          aggregateDomains(links),
	}
}

// toDTO converts a domain Link into its wire representation, mapping enums to
// their human-readable String() values and guaranteeing a non-nil Tags slice.
func toDTO(l model.Link) LinkDTO {
	tags := l.Tags
	if tags == nil {
		tags = []string{}
	}
	return LinkDTO{
		ID:          l.ID,
		URL:         l.URL,
		Title:       l.Title,
		Description: l.Description,
		Summary:     l.Summary,
		Tags:        tags,
		Domain:      domainOf(l.URL),
		Status:      l.Status.String(),
		DredgeState: l.DredgeState.String(),
		DateAdded:   l.DateAdded.Format(time.RFC3339),
	}
}

// aggregateDomains counts links per normalized host, sorted by count desc then
// alphabetically. Links whose URL yields no host are skipped.
func aggregateDomains(links []model.Link) []Count {
	counts := make(map[string]int)
	for _, l := range links {
		if d := domainOf(l.URL); d != "" {
			counts[d]++
		}
	}

	result := make([]Count, 0, len(counts))
	for d, c := range counts {
		result = append(result, Count{Label: d, Count: c})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].Label < result[j].Label
	})
	return result
}

// domainOf extracts a normalized host from a raw URL: lowercased, without a
// leading "www." and without any port. Returns "" when no host can be parsed
// (e.g. relative paths or mailto: links).
func domainOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	host := u.Hostname() // strips any :port
	if host == "" {
		return ""
	}
	host = strings.ToLower(host)
	return strings.TrimPrefix(host, "www.")
}
