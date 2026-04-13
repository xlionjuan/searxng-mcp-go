package main

import (
	"fmt"
	"strings"
)

// ============================================================================
// Formatting
// ============================================================================

// formatResults formats search results as a readable string
func formatResults(resp *SearchResponse) string {
	if len(resp.Results) == 0 {
		return "No results found."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Found %d results for '%s':\n\n", len(resp.Results), resp.Query))
	for i, r := range resp.Results {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, r.Title))
		b.WriteString(fmt.Sprintf("   URL: %s\n", r.URL))
		if r.Content != "" {
			b.WriteString(fmt.Sprintf("   Summary: %s\n", r.Content))
		}
		if r.PublishedDate != nil && *r.PublishedDate != "" {
			b.WriteString(fmt.Sprintf("   Date: %s\n", *r.PublishedDate))
		}
		b.WriteString(fmt.Sprintf("   Engine: %s\n\n", r.Engine))
	}
	return b.String()
}
