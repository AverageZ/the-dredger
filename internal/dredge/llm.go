package dredge

import (
	"context"
	"fmt"
	"strings"
)

type LLMClient interface {
	Ping() bool
	Summarize(ctx context.Context, title, description, url string, comments []string) (string, []string, error)
}

func buildSummaryPrompt(title, description, url string, comments []string) string {
	var commentsSection string
	if len(comments) > 0 {
		var b strings.Builder
		b.WriteString("\n\nCommunity Discussion (top comments):\n")
		for _, c := range comments {
			b.WriteString("- ")
			b.WriteString(c)
			b.WriteString("\n")
		}
		b.WriteString("\nInclude key insights or consensus from the community discussion in your summary.")
		commentsSection = b.String()
	}

	return fmt.Sprintf(`You are a bookmark assistant. Given a webpage's title, URL, and description, provide:
1. A concise 2-3 sentence summary of what this page is about and why someone might find it useful.
2. 3-5 relevant tags (single words or short hyphenated phrases, lowercase).

Title: %s
URL: %s
Description: %s%s

Respond in this exact format:
SUMMARY: <your summary>
TAGS: <tag1>, <tag2>, <tag3>`, title, url, description, commentsSection)
}
