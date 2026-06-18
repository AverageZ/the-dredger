package dredge

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// ResolveResult contains the outcome of URL resolution.
type ResolveResult struct {
	URL      string   // resolved article URL (or original if not resolved)
	Resolved bool     // whether URL was resolved to a different target
	Comments []string // extracted community comments (if any)
}

// Resolver detects aggregator URLs and resolves them to the underlying article URL.
type Resolver interface {
	Match(rawURL string) bool
	Resolve(ctx context.Context, client *http.Client, rawURL string, beforeFetch func(context.Context, string) error) (ResolveResult, error)
}

const tagSpan = "span"

var resolvers = []Resolver{&HNResolver{}}

// ResolveURL runs the URL through registered resolvers. If none match, it
// returns the original URL unchanged. Resolver fetch failures are returned so
// callers do not blindly retry a host that may already be rate limiting.
func ResolveURL(ctx context.Context, client *http.Client, rawURL string, beforeFetch func(context.Context, string) error) (ResolveResult, error) {
	for _, r := range resolvers {
		if r.Match(rawURL) {
			result, err := r.Resolve(ctx, client, rawURL, beforeFetch)
			if err != nil {
				return ResolveResult{URL: rawURL}, err
			}
			if !result.Resolved {
				return ResolveResult{URL: rawURL}, nil
			}
			return result, nil
		}
	}
	return ResolveResult{URL: rawURL}, nil
}

// HNResolver resolves Hacker News comment pages to their linked article URLs.
type HNResolver struct{}

func (h *HNResolver) Match(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return u.Host == "news.ycombinator.com" && u.Path == "/item" && u.Query().Get("id") != ""
}

func (h *HNResolver) Resolve(ctx context.Context, client *http.Client, rawURL string, beforeFetch func(context.Context, string) error) (ResolveResult, error) {
	if beforeFetch != nil {
		if err := beforeFetch(ctx, rawURL); err != nil {
			return ResolveResult{}, err
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return ResolveResult{}, fmt.Errorf("create HN request: %w", err)
	}
	req.Header.Set("User-Agent", "TheDredger/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return ResolveResult{}, fmt.Errorf("fetch HN page: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		return ResolveResult{}, httpStatusError(resp)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ResolveResult{}, fmt.Errorf("read HN page: %w", err)
	}

	article, ok := extractHNArticleURL(strings.NewReader(string(body)))
	if !ok {
		return ResolveResult{}, nil
	}

	comments := extractHNComments(strings.NewReader(string(body)))

	return ResolveResult{
		URL:      article,
		Resolved: true,
		Comments: comments,
	}, nil
}

const maxHNComments = 10
const maxCommentLen = 500

// extractHNComments parses HN HTML for top-level comment text from
// <span class="commtext"> elements. Returns up to maxHNComments comments,
// each truncated to maxCommentLen characters.
func extractHNComments(body io.Reader) []string {
	z := html.NewTokenizer(body)
	var comments []string
	inComment := false
	var buf strings.Builder

	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return comments
		case html.StartTagToken:
			tn, hasAttr := z.TagName()
			tag := string(tn)
			if tag == tagSpan && hasAttr {
				for {
					key, val, more := z.TagAttr()
					if string(key) == "class" && strings.Contains(string(val), "commtext") {
						inComment = true
						buf.Reset()
					}
					if !more {
						break
					}
				}
			}
		case html.EndTagToken:
			tn, _ := z.TagName()
			if string(tn) == tagSpan && inComment {
				text := strings.TrimSpace(buf.String())
				if text != "" {
					if len(text) > maxCommentLen {
						text = text[:maxCommentLen] + "..."
					}
					comments = append(comments, text)
					if len(comments) >= maxHNComments {
						return comments
					}
				}
				inComment = false
			}
		case html.TextToken:
			if inComment {
				buf.Write(z.Text())
			}
		}
	}
}

// extractHNArticleURL parses HN HTML for <span class="titleline"><a href="...">.
// Returns false for Ask HN / Show HN posts that link back to HN itself.
func extractHNArticleURL(body io.Reader) (string, bool) {
	z := html.NewTokenizer(body)
	inTitleline := false

	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return "", false
		case html.StartTagToken:
			tn, hasAttr := z.TagName()
			tag := string(tn)

			if tag == tagSpan && hasAttr {
				for {
					key, val, more := z.TagAttr()
					if string(key) == "class" && strings.Contains(string(val), "titleline") {
						inTitleline = true
					}
					if !more {
						break
					}
				}
			}

			if tag == "a" && inTitleline && hasAttr {
				for {
					key, val, more := z.TagAttr()
					if string(key) == "href" {
						href := string(val)
						// Skip internal HN links (Ask HN, Show HN, etc.)
						if strings.HasPrefix(href, "item?") {
							return "", false
						}
						return href, true
					}
					if !more {
						break
					}
				}
			}
		case html.EndTagToken:
			tn, _ := z.TagName()
			if string(tn) == tagSpan && inTitleline {
				inTitleline = false
			}
		}
	}
}
