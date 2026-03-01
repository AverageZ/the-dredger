package dredge

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHNResolverMatch(t *testing.T) {
	r := &HNResolver{}

	matches := []string{
		"https://news.ycombinator.com/item?id=12345",
		"https://news.ycombinator.com/item?id=99999999",
	}
	for _, u := range matches {
		if !r.Match(u) {
			t.Errorf("expected Match(%q) = true", u)
		}
	}

	noMatch := []string{
		"https://example.com",
		"https://news.ycombinator.com/",
		"https://news.ycombinator.com/newest",
		"https://news.ycombinator.com/item",
		"https://reddit.com/r/golang",
	}
	for _, u := range noMatch {
		if r.Match(u) {
			t.Errorf("expected Match(%q) = false", u)
		}
	}
}

const sampleHNPage = `<html><head><title>Article Title | Hacker News</title></head><body>
<table><tr class="athing">
<td class="title"><span class="titleline"><a href="https://example.com/cool-article">Article Title</a>
<span class="sitebit comhead">(<a href="from?site=example.com"><span class="sitestr">example.com</span></a>)</span></span></td>
</tr></table>
<div class="comment-tree">
<span class="commtext">This is a great article about Go performance.</span>
<span class="commtext">I disagree with the benchmarks shown here.</span>
<span class="commtext">Has anyone tried this in production?</span>
</div>
</body></html>`

func TestExtractHNArticleURL(t *testing.T) {
	article, ok := extractHNArticleURL(strings.NewReader(sampleHNPage))
	if !ok {
		t.Fatal("expected ok=true")
	}
	if article != "https://example.com/cool-article" {
		t.Errorf("got %q, want %q", article, "https://example.com/cool-article")
	}
}

const askHNPage = `<html><head><title>Ask HN: Something | Hacker News</title></head><body>
<table><tr class="athing">
<td class="title"><span class="titleline"><a href="item?id=12345">Ask HN: Something</a></span></td>
</tr></table></body></html>`

func TestExtractHNArticleURL_AskHN(t *testing.T) {
	_, ok := extractHNArticleURL(strings.NewReader(askHNPage))
	if ok {
		t.Error("expected ok=false for Ask HN post")
	}
}

func TestResolveURL_NonAggregator(t *testing.T) {
	u := "https://example.com/some-page"
	result := ResolveURL(http.DefaultClient, u)
	if result.Resolved {
		t.Error("expected Resolved=false for non-aggregator URL")
	}
	if result.URL != u {
		t.Errorf("got %q, want %q", result.URL, u)
	}
}

func TestHNResolverResolve(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, sampleHNPage)
	}))
	defer srv.Close()

	hnURL := srv.URL + "/item?id=12345"

	r := &HNResolver{}
	result, err := r.Resolve(srv.Client(), hnURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Resolved {
		t.Fatal("expected Resolved=true")
	}
	if result.URL != "https://example.com/cool-article" {
		t.Errorf("got URL %q, want %q", result.URL, "https://example.com/cool-article")
	}
	if len(result.Comments) != 3 {
		t.Fatalf("got %d comments, want 3", len(result.Comments))
	}
	if result.Comments[0] != "This is a great article about Go performance." {
		t.Errorf("got comment[0] %q", result.Comments[0])
	}
}

func TestExtractHNComments(t *testing.T) {
	html := `<html><body>
<span class="commtext">First comment here.</span>
<span class="commtext">Second comment with more detail.</span>
<span class="commtext">Third comment is short.</span>
</body></html>`

	comments := extractHNComments(strings.NewReader(html))
	if len(comments) != 3 {
		t.Fatalf("got %d comments, want 3", len(comments))
	}
	if comments[0] != "First comment here." {
		t.Errorf("got %q, want %q", comments[0], "First comment here.")
	}
	if comments[1] != "Second comment with more detail." {
		t.Errorf("got %q, want %q", comments[1], "Second comment with more detail.")
	}
}

func TestExtractHNComments_Empty(t *testing.T) {
	html := `<html><body><p>No comments on this page.</p></body></html>`
	comments := extractHNComments(strings.NewReader(html))
	if len(comments) != 0 {
		t.Errorf("got %d comments, want 0", len(comments))
	}
}

func TestExtractHNComments_Truncation(t *testing.T) {
	long := strings.Repeat("x", 600)
	html := fmt.Sprintf(`<html><body><span class="commtext">%s</span></body></html>`, long)
	comments := extractHNComments(strings.NewReader(html))
	if len(comments) != 1 {
		t.Fatalf("got %d comments, want 1", len(comments))
	}
	if len(comments[0]) != maxCommentLen+3 { // 500 + "..."
		t.Errorf("got length %d, want %d", len(comments[0]), maxCommentLen+3)
	}
}

func TestExtractHNComments_MaxLimit(t *testing.T) {
	var b strings.Builder
	b.WriteString("<html><body>")
	for i := range 15 {
		fmt.Fprintf(&b, `<span class="commtext">Comment %d</span>`, i)
	}
	b.WriteString("</body></html>")

	comments := extractHNComments(strings.NewReader(b.String()))
	if len(comments) != maxHNComments {
		t.Errorf("got %d comments, want %d", len(comments), maxHNComments)
	}
}
