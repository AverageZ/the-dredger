package search

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/alexzajac/the-dredger/internal/model"
	"github.com/alexzajac/the-dredger/internal/urlutil"
)

// snippetLen is how much summary the digest format shows. Roughly 70 tokens per
// result, so ten results cost about 700 tokens of an agent's context — cheap
// enough to search speculatively, with --full and `dredger show` for drill-down.
const snippetLen = 140

// JSONVersion is the version of the agent-facing JSON contract. It is
// deliberately separate from the web export's schema version: that one is
// coupled to the dashboard's JS, and bumping it should not break agents.
const JSONVersion = 1

// ResultDTO is the wire form of a search hit. Unlike the web export's LinkDTO it
// carries Score and omits internal dredge bookkeeping agents have no use for.
type ResultDTO struct {
	ID      int64    `json:"id"`
	URL     string   `json:"url"`
	Title   string   `json:"title"`
	Summary string   `json:"summary,omitempty"`
	Tags    []string `json:"tags"`
	Domain  string   `json:"domain"`
	Score   float64  `json:"score"`
	Loose   bool     `json:"loose,omitempty"`
}

// ResponseDTO is the top-level JSON emitted by `dredger search`.
type ResponseDTO struct {
	Version        int         `json:"version"`
	Query          string      `json:"query"`
	Count          int         `json:"count"`
	Loose          bool        `json:"loose"`
	Results        []ResultDTO `json:"results"`
	SuggestedTags  []string    `json:"suggestedTags,omitempty"`
	TotalSearched  int         `json:"totalSearched"`
	EnrichedOfType int         `json:"enrichedCount"`
}

func toResultDTO(r Result) ResultDTO {
	tags := r.Link.Tags
	if tags == nil {
		tags = []string{}
	}
	return ResultDTO{
		ID:      r.Link.ID,
		URL:     r.Link.URL,
		Title:   r.Link.Title,
		Summary: r.Link.Summary,
		Tags:    tags,
		Domain:  urlutil.Domain(r.Link.URL),
		Score:   round2(r.Score),
		Loose:   r.Loose,
	}
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}

// WriteJSON emits the machine-readable response. This shape is the stable
// contract; the human format below is not.
func WriteJSON(w io.Writer, query string, results []Result, suggested []string, totalSearched, enriched int) error {
	dtos := make([]ResultDTO, 0, len(results))
	loose := false
	for _, r := range results {
		if r.Loose {
			loose = true
		}
		dtos = append(dtos, toResultDTO(r))
	}
	resp := ResponseDTO{
		Version:        JSONVersion,
		Query:          query,
		Count:          len(dtos),
		Loose:          loose,
		Results:        dtos,
		SuggestedTags:  suggested,
		TotalSearched:  totalSearched,
		EnrichedOfType: enriched,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}

// WriteDigest emits the human/agent-readable listing. When results are loose it
// says so up front and offers the suggested tags, so the reader understands the
// difference between "nothing matched well" and "nothing exists".
func WriteDigest(w io.Writer, query string, results []Result, suggested []string, full bool) error {
	if len(results) == 0 {
		if err := writef(w, "No matches for %q.\n", query); err != nil {
			return err
		}
		if len(suggested) > 0 {
			return writef(w, "Related tags in your bookmarks: %s\n", strings.Join(suggested, ", "))
		}
		return nil
	}

	if results[0].Loose {
		if err := writef(w, "No strong matches for %q — closest %d:\n\n", query, len(results)); err != nil {
			return err
		}
	}

	for _, r := range results {
		if err := writef(w, "#%-5d %5.2f  %s\n", r.Link.ID, r.Score, titleOf(r.Link)); err != nil {
			return err
		}
		if len(r.Link.Tags) > 0 {
			if err := writef(w, "             [%s]\n", strings.Join(r.Link.Tags, ", ")); err != nil {
				return err
			}
		}
		if err := writef(w, "             %s\n", r.Link.URL); err != nil {
			return err
		}
		if s := summaryOf(r.Link, full); s != "" {
			if err := writef(w, "             %s\n", s); err != nil {
				return err
			}
		}
		if err := writef(w, "\n"); err != nil {
			return err
		}
	}

	if results[0].Loose && len(suggested) > 0 {
		return writef(w, "Try these tags instead: %s\n", strings.Join(suggested, ", "))
	}
	return nil
}

// WriteLink renders a single link in full, for `dredger show`.
func WriteLink(w io.Writer, l model.Link) error {
	if err := writef(w, "#%d  %s\n", l.ID, titleOf(l)); err != nil {
		return err
	}
	if err := writef(w, "URL:     %s\n", l.URL); err != nil {
		return err
	}
	if len(l.Tags) > 0 {
		if err := writef(w, "Tags:    %s\n", strings.Join(l.Tags, ", ")); err != nil {
			return err
		}
	}
	if err := writef(w, "Added:   %s\n", l.DateAdded.Format("2006-01-02")); err != nil {
		return err
	}
	if err := writef(w, "Status:  %s\n", l.Status); err != nil {
		return err
	}
	if l.Description != "" {
		if err := writef(w, "\nDescription:\n%s\n", l.Description); err != nil {
			return err
		}
	}
	if l.Summary != "" {
		if err := writef(w, "\nSummary:\n%s\n", l.Summary); err != nil {
			return err
		}
	}
	if !l.Enriched {
		return writef(w, "\n(not yet dredged — no AI summary or tags)\n")
	}
	return nil
}

func writef(w io.Writer, format string, args ...any) error {
	_, err := fmt.Fprintf(w, format, args...)
	return err
}

// LinkDTO is the JSON form of a single link for `dredger show --json`.
type LinkDTO struct {
	ID          int64    `json:"id"`
	URL         string   `json:"url"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	Tags        []string `json:"tags"`
	Domain      string   `json:"domain"`
	Status      string   `json:"status"`
	Enriched    bool     `json:"enriched"`
	DateAdded   string   `json:"dateAdded"`
}

// WriteLinkJSON emits a single link as JSON.
func WriteLinkJSON(w io.Writer, l model.Link) error {
	tags := l.Tags
	if tags == nil {
		tags = []string{}
	}
	dto := LinkDTO{
		ID:          l.ID,
		URL:         l.URL,
		Title:       l.Title,
		Description: l.Description,
		Summary:     l.Summary,
		Tags:        tags,
		Domain:      urlutil.Domain(l.URL),
		Status:      l.Status.String(),
		Enriched:    l.Enriched,
		DateAdded:   l.DateAdded.Format("2006-01-02T15:04:05Z07:00"),
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(dto)
}

// titleOf falls back to the domain when a link has never been dredged, so an
// un-enriched result is still identifiable instead of showing a blank line.
func titleOf(l model.Link) string {
	if t := strings.TrimSpace(l.Title); t != "" {
		return t
	}
	if d := urlutil.Domain(l.URL); d != "" {
		return "(untitled — " + d + ")"
	}
	return "(untitled)"
}

// summaryOf returns the summary, truncated for the digest view. It falls back to
// the scraped description when the AI summary is missing.
func summaryOf(l model.Link, full bool) string {
	s := strings.TrimSpace(l.Summary)
	if s == "" {
		s = strings.TrimSpace(l.Description)
	}
	s = strings.Join(strings.Fields(s), " ") // collapse newlines for one-line display
	if full || len(s) <= snippetLen {
		return s
	}
	end := snippetLen
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	cut := s[:end]
	if i := strings.LastIndex(cut, " "); i > snippetLen/2 {
		cut = cut[:i]
	}
	return cut + "…"
}
