// Package urlutil holds small URL helpers shared across packages that need to
// reason about a link's host without pulling in a full parser each time.
package urlutil

import (
	"net/url"
	"strings"
)

// Domain extracts a normalized host from a raw URL: lowercased, without a
// leading "www." and without any port. Returns "" when no host can be parsed
// (e.g. relative paths or mailto: links).
func Domain(raw string) string {
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

var pathSeparators = strings.NewReplacer("/", " ", "-", " ", "_", " ", ".", " ", "+", " ", "=", " ", "&", " ")

// Path returns the lowercased path and query of a URL with separators replaced
// by spaces, so callers can token-match against slugs like
// "/blog/wave-function-collapse" without re-parsing.
func Path(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	s := u.Path
	if u.RawQuery != "" {
		query, err := url.QueryUnescape(u.RawQuery)
		if err != nil {
			query = u.RawQuery
		}
		s += " " + query
	}
	s = strings.ToLower(s)
	return pathSeparators.Replace(s)
}
