// Package search ranks links against a free-text query using field weights
// tuned for the dredger corpus: LLM-extracted tags count for far more than
// summary prose, and covering every query term counts for more than repeating
// one of them.
//
// It scores in memory rather than in SQL. At this corpus size (hundreds of
// links, a few hundred KB of text) a full scan is sub-millisecond, and scoring
// in Go buys per-field weighting and a sane zero-result path that neither LIKE
// nor FTS5 gives for free.
package search

import (
	"sort"
	"strings"

	"github.com/alexzajac/the-dredger/internal/model"
	"github.com/alexzajac/the-dredger/internal/urlutil"
)

// Options controls a search.
type Options struct {
	Query string

	// Tags boost matching links rather than filtering them. The tag vocabulary
	// is long-tail (roughly two links per tag), so AND-filtering on tags returns
	// nothing far more often than it returns something useful. Set TagStrict to
	// filter instead.
	Tags      []string
	TagStrict bool

	// Domain, when set, restricts results to links on that host.
	Domain string

	// Limit caps the returned results. Zero means unlimited.
	Limit int

	// IncludePruned keeps links the user has already pruned. Callers normally
	// pass a pre-filtered slice, so this only matters when the slice contains
	// pruned links.
	IncludePruned bool
}

// Result is one ranked link.
type Result struct {
	Link    model.Link
	Score   float64
	Matched []string // query terms that contributed
	Loose   bool     // below threshold or missing a query term
}

// tagBoost is added per matching tag when Options.Tags is set without TagStrict.
const tagBoost = 8.0

// looseFallbackCount is how many weak matches to surface when no link makes a
// strong match. Returning an empty list would let a caller conclude "no
// bookmarks exist on this topic", which is usually wrong and always unhelpful.
const looseFallbackCount = 5

// Search ranks links against opts and returns them best-first.
//
// Strong results must cover every query term. When no link does, it returns up
// to looseFallbackCount partial or below-threshold matches flagged Loose so the
// caller can present them as suggestions instead of showing nothing.
func Search(links []model.Link, opts Options) []Result {
	terms := queryTerms(opts.Query)
	wantTags := normalizeTags(opts.Tags)
	wantDomain := strings.ToLower(strings.TrimSpace(opts.Domain))
	if domain := urlutil.Domain("https://" + wantDomain); domain != "" {
		wantDomain = domain
	}

	// With no query and no tag filter there is nothing to rank by.
	if len(terms) == 0 && len(wantTags) == 0 {
		return nil
	}

	var strong, loose []Result
	for _, l := range links {
		if l.Status == model.Pruned && !opts.IncludePruned {
			continue
		}
		if wantDomain != "" && urlutil.Domain(l.URL) != wantDomain {
			continue
		}

		doc := buildDoc(l)

		tagHits := countTagHits(doc, wantTags)
		if opts.TagStrict && len(wantTags) > 0 && tagHits < len(wantTags) {
			continue
		}

		score, matched := scoreLink(doc, terms)
		score += float64(tagHits) * tagBoost
		if score <= 0 {
			continue
		}

		r := Result{Link: l, Score: score, Matched: matched}
		// A repeated broad term can otherwise overcome the coverage penalty and
		// make a weak multi-term query look like a confident result.
		if len(matched) == len(terms) && score >= looseThreshold {
			strong = append(strong, r)
		} else {
			loose = append(loose, r)
		}
	}

	results := strong
	if len(results) == 0 {
		for i := range loose {
			loose[i].Loose = true
		}
		results = loose
		sortResults(results)
		if len(results) > looseFallbackCount {
			results = results[:looseFallbackCount]
		}
		return limitResults(results, opts.Limit)
	}

	sortResults(results)
	return limitResults(results, opts.Limit)
}

func limitResults(results []Result, limit int) []Result {
	if limit > 0 && len(results) > limit {
		return results[:limit]
	}
	return results
}

// sortResults orders by score desc, then enriched links first (they carry the
// summary and tags that make a result actually readable), then newest first,
// then by ID so the order is stable across runs.
func sortResults(rs []Result) {
	sort.SliceStable(rs, func(i, j int) bool {
		a, b := rs[i], rs[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Link.Enriched != b.Link.Enriched {
			return a.Link.Enriched
		}
		if !a.Link.DateAdded.Equal(b.Link.DateAdded) {
			return a.Link.DateAdded.After(b.Link.DateAdded)
		}
		return a.Link.ID < b.Link.ID
	})
}

// normalizeTags lowercases and trims requested tags, dropping empties.
func normalizeTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// countTagHits counts how many of the wanted tags the link carries, matching
// either exactly or on the stemmed tag words.
func countTagHits(d docFields, want []string) int {
	if len(want) == 0 {
		return 0
	}
	hits := 0
	for _, w := range want {
		found := false
		for _, t := range d.tags {
			if t == w {
				found = true
				break
			}
		}
		if !found {
			ws := stem(w)
			for _, t := range d.tagTokens {
				if t == ws {
					found = true
					break
				}
			}
		}
		if found {
			hits++
		}
	}
	return hits
}

// SuggestTags returns up to n tags from the corpus that are lexically close to
// the query, ranked by closeness then frequency. It backs the "try these
// instead" hint on a weak search: with a sparse, uncontrolled tag vocabulary,
// telling the caller what words the corpus actually uses is more useful than
// telling them their query found nothing.
func SuggestTags(links []model.Link, query string, n int) []string {
	terms := queryTerms(query)
	if len(terms) == 0 || n <= 0 {
		return nil
	}

	type tagScore struct {
		tag   string
		score float64
		count int
	}

	counts := make(map[string]int)
	for _, l := range links {
		if l.Status == model.Pruned {
			continue
		}
		for _, t := range l.Tags {
			t = strings.ToLower(strings.TrimSpace(t))
			if t != "" {
				counts[t]++
			}
		}
	}

	scored := make([]tagScore, 0, len(counts))
	for tag, count := range counts {
		words := stemAll(tokenize(tag))
		var s float64
		for _, term := range terms {
			for _, w := range words {
				switch {
				case w == term:
					s += 3
				case len(term) >= 3 && strings.HasPrefix(w, term):
					s += 2
				case len(w) >= 3 && strings.HasPrefix(term, w):
					s += 1
				}
			}
		}
		if s > 0 {
			scored = append(scored, tagScore{tag: tag, score: s, count: count})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if scored[i].count != scored[j].count {
			return scored[i].count > scored[j].count
		}
		return scored[i].tag < scored[j].tag
	})

	if len(scored) > n {
		scored = scored[:n]
	}
	out := make([]string, len(scored))
	for i, s := range scored {
		out[i] = s.tag
	}
	return out
}
