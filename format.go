package main

import (
	"fmt"
	"html"
	"strings"
	"unicode/utf8"
)

// ============================================================================
// Formatting
// ============================================================================

// unescapeIfNeeded calls html.UnescapeString only when the string contains
// HTML entities, avoiding unnecessary allocations.
func unescapeIfNeeded(s string) string {
	if !strings.ContainsAny(s, "&<>\"'") {
		return s
	}
	return html.UnescapeString(s)
}

// formatResults formats search results as a readable string
func formatResults(resp *SearchResponse) string {
	if resp == nil {
		return "No results found."
	}
	if len(resp.Results) == 0 {
		return "No results found."
	}

	var b strings.Builder
	total := resp.NumberOfResults
	if total == 0 {
		total = len(resp.Results)
	}
	b.WriteString(fmt.Sprintf("Found %d results for '%s':\n\n", total, resp.Query))
	for i, r := range resp.Results {
		title := unescapeIfNeeded(r.Title)
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, title))
		b.WriteString(fmt.Sprintf("   URL: %s\n", r.URL))
		if r.Content != "" {
			content := unescapeIfNeeded(r.Content)
			if utf8.RuneCountInString(content) > MaxContentRunes {
				runes := []rune(content)
				content = string(runes[:MaxContentRunes])
			}
			b.WriteString(fmt.Sprintf("   Summary: %s\n", content))
		}
		if r.PublishedDate != nil && *r.PublishedDate != "" {
			b.WriteString(fmt.Sprintf("   Date: %s\n", *r.PublishedDate))
		}
		b.WriteString(fmt.Sprintf("   Engine: %s\n\n", r.Engine))
	}
	return b.String()
}
