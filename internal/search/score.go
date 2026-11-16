package search

import (
	"strings"
	"unicode"

	"github.com/alexzajac/the-dredger/internal/model"
	"github.com/alexzajac/the-dredger/internal/urlutil"
)

// Field weights. Tags are worth 5x a summary hit on purpose: they are a concept
// vocabulary the dredge pipeline's LLM already extracted, so a tag match is a
// statement about what a link is *about*, while summary prose only says the word
// appeared. Raw text matching cannot reach that signal, which is the whole
// reason this scorer exists instead of a SQL LIKE.
const (
	weightTagExact    = 10.0
	weightTagPrefix   = 6.0
	weightTitleExact  = 5.0
	weightTitlePrefix = 3.0
	weightSummary     = 2.0
	weightDescription = 1.0
	weightURL         = 1.0
)

// summaryHitCap bounds how many times a single term may score from the summary.
// Summaries average ~380 characters, so without a cap a long rambling summary
// that repeats one word outranks a link precisely tagged with the query.
const summaryHitCap = 3

// looseThreshold is the score below which a result is considered a weak match
// and is only surfaced as a "closest match" suggestion.
//
// It sits at one summary mention (weightSummary) on purpose: a link whose body
// text actually says the word is a real hit, while a link matching only in its
// URL or scraped description is not. The threshold can stay this low because the
// coverage-squared factor already does the filtering for multi-term queries —
// a link matching one term of "terminal ui" scores 2.0*(1/2)^2 = 0.5 and falls
// below it, while one matching both scores 4.0 and clears it.
const looseThreshold = weightSummary

// stopwords are dropped from queries. Deliberately tiny: this corpus is
// technical, so aggressive stopword removal costs more than it saves.
var stopwords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "by": true, "for": true, "from": true, "how": true, "in": true,
	"is": true, "it": true, "of": true, "on": true, "or": true, "that": true,
	"the": true, "to": true, "with": true,
}

// tokenize splits arbitrary text into lowercase word tokens on any non-alphanumeric
// boundary, so "wave-function-collapse" and "wave function collapse" agree.
func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// queryTerms tokenizes a query and drops stopwords, stemming what remains. If
// the query is nothing but stopwords the terms are kept as-is rather than
// returning nothing, so "how to" still searches for something.
func queryTerms(q string) []string {
	toks := tokenize(q)
	kept := make([]string, 0, len(toks))
	for _, t := range toks {
		if !stopwords[t] {
			kept = append(kept, stem(t))
		}
	}
	if len(kept) == 0 {
		for _, t := range toks {
			kept = append(kept, stem(t))
		}
	}
	return kept
}

// stem is a deliberately dumb suffix stripper. It exists only so "agents"
// matches "agent" and "rendering" matches "render". It is applied to both the
// query and the document, so its mistakes are at least symmetric. Anything
// smarter belongs in a real stemmer, and this corpus does not justify one.
func stem(w string) string {
	switch {
	case len(w) > 4 && strings.HasSuffix(w, "ies"):
		return w[:len(w)-3] + "y"
	case len(w) > 4 && strings.HasSuffix(w, "ing"):
		return w[:len(w)-3]
	case len(w) > 4 && strings.HasSuffix(w, "es"):
		return w[:len(w)-2]
	case len(w) > 3 && strings.HasSuffix(w, "ed"):
		return w[:len(w)-2]
	case len(w) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss"):
		return w[:len(w)-1]
	default:
		return w
	}
}

// stemAll stems every token in a slice.
func stemAll(toks []string) []string {
	out := make([]string, len(toks))
	for i, t := range toks {
		out[i] = stem(t)
	}
	return out
}

// docFields is the pre-tokenized, pre-stemmed form of a link. Building it once
// per link per search keeps scoring O(terms) instead of re-tokenizing 380-char
// summaries for every query term.
type docFields struct {
	tags        []string // raw, lowercased, not stemmed — for exact tag equality
	tagTokens   []string // stemmed tag words
	title       []string
	summary     []string
	description []string
	url         []string
}

func buildDoc(l model.Link) docFields {
	d := docFields{
		title:       stemAll(tokenize(l.Title)),
		summary:     stemAll(tokenize(l.Summary)),
		description: stemAll(tokenize(l.Description)),
	}
	for _, t := range l.Tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		d.tags = append(d.tags, t)
		d.tagTokens = append(d.tagTokens, stemAll(tokenize(t))...)
	}
	urlText := urlutil.Domain(l.URL) + " " + urlutil.Path(l.URL)
	d.url = stemAll(tokenize(urlText))
	return d
}

// scoreTerm returns the score a single stemmed query term earns against a doc.
func scoreTerm(d docFields, term string) float64 {
	var score float64

	// Exact tag equality: the query term IS one of the link's concepts.
	for _, t := range d.tags {
		if t == term || stem(t) == term {
			score += weightTagExact
			break
		}
	}
	// Tag word / prefix match, e.g. "proc" against the tag "proc-gen".
	if score == 0 {
		for _, t := range d.tagTokens {
			if t == term {
				score += weightTagExact
				break
			}
			if len(term) >= 3 && strings.HasPrefix(t, term) {
				score += weightTagPrefix
				break
			}
		}
	}

	titleScore := 0.0
	for _, w := range d.title {
		if w == term {
			titleScore = weightTitleExact
			break
		}
		if len(term) >= 3 && strings.HasPrefix(w, term) && titleScore < weightTitlePrefix {
			titleScore = weightTitlePrefix
		}
	}
	score += titleScore

	hits := 0
	for _, w := range d.summary {
		if w == term {
			hits++
			if hits == summaryHitCap {
				break
			}
		}
	}
	score += float64(hits) * weightSummary

	for _, w := range d.description {
		if w == term {
			score += weightDescription
			break
		}
	}

	for _, w := range d.url {
		if w == term {
			score += weightURL
			break
		}
	}

	return score
}

// scoreLink scores a link against every query term and returns the total plus
// the terms that contributed. The coverage factor is squared so that matching
// all of "procedural generation roguelike" beats matching only "generation"
// three times over — which is exactly the difference between a useful result
// and noise for a conceptual query.
func scoreLink(d docFields, terms []string) (float64, []string) {
	if len(terms) == 0 {
		return 0, nil
	}

	var total float64
	matched := make([]string, 0, len(terms))
	for _, term := range terms {
		if s := scoreTerm(d, term); s > 0 {
			total += s
			matched = append(matched, term)
		}
	}
	if total == 0 {
		return 0, nil
	}

	coverage := float64(len(matched)) / float64(len(terms))
	return total * coverage * coverage, matched
}
