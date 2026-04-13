package main

import (
	"fmt"
)

// ============================================================================
// Formatting
// ============================================================================

// formatResults formats search results as a readable string
func formatResults(resp *SearchResponse) string {
	inferDates(resp)
	if len(resp.Results) == 0 {
		return "No results found."
	}

	output := fmt.Sprintf("Found %d results for '%s':\n\n", len(resp.Results), resp.Query)
	for i, r := range resp.Results {
		output += fmt.Sprintf("%d. %s\n", i+1, r.Title)
		output += fmt.Sprintf("   URL: %s\n", r.URL)
		if r.Content != "" {
			output += fmt.Sprintf("   Summary: %s\n", r.Content)
		}
		if r.PublishedDate != nil && *r.PublishedDate != "" {
			output += fmt.Sprintf("   Date: %s\n", *r.PublishedDate)
		}
		output += fmt.Sprintf("   Engine: %s\n\n", r.Engine)
	}
	return output
}
