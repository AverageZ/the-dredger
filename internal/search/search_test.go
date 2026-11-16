package search

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alexzajac/the-dredger/internal/model"
)

const (
	queryShader    = "shader"
	queryRoguelike = "roguelike"
	looseQuery     = "game feel juice"
	weakResultURL  = "https://example.com/x"
)

func link(id int64, title, summary string, tags ...string) model.Link {
	return model.Link{
		ID:        id,
		URL:       "https://example.com/l/" + title,
		Title:     title,
		Summary:   summary,
		Tags:      tags,
		Enriched:  true,
		DateAdded: time.Date(2026, 1, int(id%28)+1, 0, 0, 0, 0, time.UTC),
	}
}

func ids(rs []Result) []int64 {
	out := make([]int64, len(rs))
	for i, r := range rs {
		out[i] = r.Link.ID
	}
	return out
}

func TestSearchFieldWeighting(t *testing.T) {
	tagged := link(1, "Some Unrelated Page", "nothing here", queryShader)
	titled := link(2, "Shader Basics", "nothing here")
	summarized := link(3, "Unrelated", "a shader is a program")

	got := Search([]model.Link{summarized, titled, tagged}, Options{Query: queryShader})

	want := []int64{1, 2, 3} // tag > title > summary
	if len(got) != 3 {
		t.Fatalf("got %d results, want 3: %v", len(got), ids(got))
	}
	for i, id := range want {
		if got[i].Link.ID != id {
			t.Errorf("rank %d = link %d, want %d (order was %v)", i, got[i].Link.ID, id, ids(got))
		}
	}
}

func TestSearchCoverageBeatsRepetition(t *testing.T) {
	// One term repeated in the summary vs. both terms covered by tags.
	repeater := link(1, "Generation", "generation generation generation generation")
	coverer := link(2, "A Dungeon Post", "", "procedural", "generation")

	got := Search([]model.Link{repeater, coverer}, Options{Query: "procedural generation"})

	if len(got) == 0 {
		t.Fatal("expected a full-coverage result")
	}
	if got[0].Link.ID != 2 {
		t.Errorf("full-coverage link should rank first, got order %v", ids(got))
	}
	if len(got[0].Matched) != 2 {
		t.Errorf("expected both terms matched, got %v", got[0].Matched)
	}
}

func TestSummaryHitCap(t *testing.T) {
	few := link(1, "", queryShader)
	many := link(2, "", strings.Repeat(queryShader+" ", 20))

	fewDoc, manyDoc := buildDoc(few), buildDoc(many)
	fewScore := scoreTerm(fewDoc, queryShader)
	manyScore := scoreTerm(manyDoc, queryShader)

	if manyScore > fewScore*float64(summaryHitCap) {
		t.Errorf("summary score uncapped: 20 mentions scored %v vs %v for one (cap=%d)",
			manyScore, fewScore, summaryHitCap)
	}
	if manyScore != weightSummary*summaryHitCap {
		t.Errorf("expected capped score %v, got %v", weightSummary*summaryHitCap, manyScore)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	links := []model.Link{link(1, "Shader Basics", "")}
	for _, q := range []string{"", "   ", "\t"} {
		if got := Search(links, Options{Query: q}); got != nil {
			t.Errorf("Search(%q) = %v, want nil", q, ids(got))
		}
	}
}

func TestSearchStopwordOnlyQuery(t *testing.T) {
	// "how to" is all stopwords; we keep them rather than searching for nothing.
	links := []model.Link{link(1, "How To Build A Shader", "")}
	got := Search(links, Options{Query: "how to"})
	if len(got) == 0 {
		t.Error("stopword-only query returned nothing; expected the fallback to keep the terms")
	}
}

func TestSearchLooseFallback(t *testing.T) {
	// Only a weak URL/description-level hit — below looseThreshold.
	weak := model.Link{
		ID:          1,
		URL:         weakResultURL,
		Title:       "Unrelated",
		Description: "mentions juice once",
		Tags:        []string{"cooking"},
		DateAdded:   time.Now(),
	}

	got := Search([]model.Link{weak}, Options{Query: looseQuery})

	if len(got) == 0 {
		t.Fatal("expected loose fallback results, got none")
	}
	if !got[0].Loose {
		t.Errorf("expected Loose=true, got score %v", got[0].Score)
	}
}

func TestSearchPartialMatchesAreLoose(t *testing.T) {
	const partialTerm = "game"
	partial := link(1, partialTerm, strings.Repeat(partialTerm+" ", summaryHitCap+10), partialTerm)

	got := Search([]model.Link{partial}, Options{Query: looseQuery})
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	if !got[0].Loose {
		t.Errorf("partial match with score %.2f should be loose", got[0].Score)
	}
}

func TestSearchLooseFallbackIsCapped(t *testing.T) {
	var links []model.Link
	for i := int64(1); i <= 20; i++ {
		links = append(links, model.Link{
			ID:          i,
			URL:         weakResultURL,
			Description: "juice",
			DateAdded:   time.Now(),
		})
	}
	got := Search(links, Options{Query: looseQuery})
	if len(got) > looseFallbackCount {
		t.Errorf("loose fallback returned %d, want at most %d", len(got), looseFallbackCount)
	}
}

func TestSearchLooseFallbackHonorsLimit(t *testing.T) {
	links := make([]model.Link, 0, looseFallbackCount)
	for i := int64(1); i <= looseFallbackCount; i++ {
		links = append(links, model.Link{
			ID:          i,
			URL:         weakResultURL,
			Description: "juice",
			DateAdded:   time.Now(),
		})
	}

	got := Search(links, Options{Query: looseQuery, Limit: 2})
	if len(got) != 2 {
		t.Errorf("fallback Limit=2 returned %d results", len(got))
	}
}

func TestSearchNoResultsAtAll(t *testing.T) {
	links := []model.Link{link(1, "Shader Basics", "graphics", "graphics")}
	if got := Search(links, Options{Query: "quantum knitting"}); len(got) != 0 {
		t.Errorf("expected no results, got %v", ids(got))
	}
}

func TestSearchTagBoostVsStrict(t *testing.T) {
	withTag := link(1, "Post About Things", "", queryRoguelike)
	withoutTag := link(2, "Roguelike Design Notes", "")

	boosted := Search([]model.Link{withTag, withoutTag}, Options{
		Query: queryRoguelike, Tags: []string{queryRoguelike},
	})
	if len(boosted) != 2 {
		t.Fatalf("boost mode should keep both links, got %v", ids(boosted))
	}
	if boosted[0].Link.ID != 1 {
		t.Errorf("tagged link should be boosted to first, got %v", ids(boosted))
	}

	strict := Search([]model.Link{withTag, withoutTag}, Options{
		Query: queryRoguelike, Tags: []string{queryRoguelike}, TagStrict: true,
	})
	if len(strict) != 1 || strict[0].Link.ID != 1 {
		t.Errorf("strict mode should keep only the tagged link, got %v", ids(strict))
	}
}

func TestSearchExcludesPruned(t *testing.T) {
	pruned := link(1, "Shader Basics", "")
	pruned.Status = model.Pruned
	kept := link(2, "Shader Advanced", "")

	got := Search([]model.Link{pruned, kept}, Options{Query: queryShader})
	if len(got) != 1 || got[0].Link.ID != 2 {
		t.Errorf("pruned link should be excluded, got %v", ids(got))
	}

	withPruned := Search([]model.Link{pruned, kept}, Options{Query: queryShader, IncludePruned: true})
	if len(withPruned) != 2 {
		t.Errorf("IncludePruned should keep both, got %v", ids(withPruned))
	}
}

func TestSearchDomainFilter(t *testing.T) {
	a := link(1, "Shader One", "")
	a.URL = "https://good.com/a"
	b := link(2, "Shader Two", "")
	b.URL = "https://other.com/b"

	for _, domain := range []string{"good.com", "www.good.com", " GOOD.COM:443 "} {
		got := Search([]model.Link{a, b}, Options{Query: queryShader, Domain: domain})
		if len(got) != 1 || got[0].Link.ID != 1 {
			t.Errorf("domain filter %q failed, got %v", domain, ids(got))
		}
	}
}

func TestSearchLimit(t *testing.T) {
	var links []model.Link
	for i := int64(1); i <= 10; i++ {
		links = append(links, link(i, "Shader Post", "", queryShader))
	}
	got := Search(links, Options{Query: queryShader, Limit: 3})
	if len(got) != 3 {
		t.Errorf("Limit=3 returned %d results", len(got))
	}
}

func TestSearchStemming(t *testing.T) {
	tests := []struct {
		query string
		title string
	}{
		{"agents", "LLM Agent Design"},
		{"agent", "LLM Agents Design"},
		{"rendering", "Render Pipelines"},
		{"libraries", "A Library Of Things"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := Search([]model.Link{link(1, tt.title, "")}, Options{Query: tt.query})
			if len(got) == 0 {
				t.Errorf("query %q did not match title %q", tt.query, tt.title)
			}
		})
	}
}

func TestSearchHyphenatedTagMatch(t *testing.T) {
	l := link(1, "Untitled", "", "wave-function-collapse")
	got := Search([]model.Link{l}, Options{Query: "wave function collapse"})
	if len(got) == 0 {
		t.Fatal("hyphenated tag should match the spaced query")
	}
	if got[0].Loose {
		t.Errorf("full tag coverage should be a strong match, scored %v", got[0].Score)
	}
}

func TestSearchIsDeterministic(t *testing.T) {
	links := []model.Link{
		link(3, "Shader C", "", queryShader),
		link(1, "Shader A", "", queryShader),
		link(2, "Shader B", "", queryShader),
	}
	first := ids(Search(links, Options{Query: queryShader}))
	for i := 0; i < 5; i++ {
		if got := ids(Search(links, Options{Query: queryShader})); !equal(got, first) {
			t.Fatalf("unstable ordering: %v then %v", first, got)
		}
	}
}

func equal(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSuggestTags(t *testing.T) {
	links := []model.Link{
		link(1, "", "", "game-development", "godot"),
		link(2, "", "", "game-development"),
		link(3, "", "", "gamedev"),
		link(4, "", "", "cooking"),
	}

	got := SuggestTags(links, "game feel", 3)
	if len(got) == 0 {
		t.Fatal("expected tag suggestions")
	}
	if got[0] != "game-development" {
		t.Errorf("expected the most common game tag first, got %v", got)
	}
	for _, tag := range got {
		if tag == "cooking" {
			t.Errorf("unrelated tag suggested: %v", got)
		}
	}
}

func TestSuggestTagsEmptyQuery(t *testing.T) {
	links := []model.Link{link(1, "", "", "go")}
	if got := SuggestTags(links, "", 5); got != nil {
		t.Errorf("expected nil for empty query, got %v", got)
	}
}

func TestStem(t *testing.T) {
	tests := map[string]string{
		"agents":    "agent",
		"libraries": "library",
		"rendering": "render",
		"rendered":  "render",
		"shaders":   "shader",
		"css":       "css", // -ss is not a plural
		"go":        "go",  // too short to strip
		"is":        "is",  // too short to strip
		"processes": "process",
	}
	for in, want := range tests {
		if got := stem(in); got != want {
			t.Errorf("stem(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTokenize(t *testing.T) {
	got := tokenize("Wave-Function_Collapse (v2.0)!")
	want := []string{"wave", "function", "collapse", "v2", "0"}
	if len(got) != len(want) {
		t.Fatalf("tokenize gave %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWriteJSONShape(t *testing.T) {
	results := []Result{{Link: link(1, "Shader Basics", "About shaders.", "shader"), Score: 12.5}}

	var buf bytes.Buffer
	if err := WriteJSON(&buf, "shader", results, nil, 100, 90); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var resp ResponseDTO
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if resp.Version != JSONVersion {
		t.Errorf("version = %d, want %d", resp.Version, JSONVersion)
	}
	if resp.Count != 1 || len(resp.Results) != 1 {
		t.Fatalf("count = %d, results = %d", resp.Count, len(resp.Results))
	}
	r := resp.Results[0]
	if r.ID != 1 || r.Score != 12.5 || r.Domain != "example.com" {
		t.Errorf("unexpected result DTO: %+v", r)
	}
	if resp.TotalSearched != 100 || resp.EnrichedOfType != 90 {
		t.Errorf("corpus counts not carried through: %+v", resp)
	}
}

func TestWriteJSONNeverNullTags(t *testing.T) {
	l := model.Link{ID: 1, URL: "https://example.com", Tags: nil}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, "q", []Result{{Link: l}}, nil, 1, 0); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if strings.Contains(buf.String(), `"tags": null`) {
		t.Errorf("tags serialized as null:\n%s", buf.String())
	}
}

func TestWriteDigestLooseAnnouncesItself(t *testing.T) {
	results := []Result{{Link: link(1, "Something", ""), Score: 1.0, Loose: true}}

	var buf bytes.Buffer
	if err := WriteDigest(&buf, "game feel juice", results, []string{"gamedev", "ux"}, false); err != nil {
		t.Fatalf("WriteDigest: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "No strong matches") {
		t.Errorf("loose results not labeled:\n%s", out)
	}
	if !strings.Contains(out, "gamedev") {
		t.Errorf("suggested tags missing:\n%s", out)
	}
}

func TestWriteDigestEmptyStillHelps(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteDigest(&buf, "quantum knitting", nil, []string{"knitting"}, false); err != nil {
		t.Fatalf("WriteDigest: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "No matches") {
		t.Errorf("missing no-match message:\n%s", out)
	}
	if !strings.Contains(out, "knitting") {
		t.Errorf("empty result should still offer related tags:\n%s", out)
	}
}

func TestSummaryOfTruncates(t *testing.T) {
	long := strings.Repeat("word ", 100)
	l := model.Link{Summary: long}

	short := summaryOf(l, false)
	if len(short) > snippetLen+3 {
		t.Errorf("snippet too long: %d chars", len(short))
	}
	if !strings.HasSuffix(short, "…") {
		t.Errorf("truncated snippet should be elided, got %q", short)
	}
	if full := summaryOf(l, true); len(full) < len(long)-5 {
		t.Errorf("full mode truncated: %d vs %d", len(full), len(long))
	}
}

func TestSummaryOfPreservesUTF8(t *testing.T) {
	long := strings.Repeat("界", snippetLen)
	l := model.Link{Summary: long}
	want := strings.Repeat("界", snippetLen/len("界")) + "…"
	if got := summaryOf(l, false); got != want {
		t.Errorf("UTF-8 snippet = %q, want %q", got, want)
	}
	if got := summaryOf(l, true); got != long {
		t.Errorf("full summary changed: %q", got)
	}
}

func TestSummaryOfFallsBackToDescription(t *testing.T) {
	l := model.Link{Description: "scraped description"}
	if got := summaryOf(l, false); got != "scraped description" {
		t.Errorf("expected description fallback, got %q", got)
	}
}

func TestTitleOfFallsBackToDomain(t *testing.T) {
	l := model.Link{URL: "https://www.example.com/page"}
	got := titleOf(l)
	if !strings.Contains(got, "example.com") {
		t.Errorf("untitled link should fall back to its domain, got %q", got)
	}
}
