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

// formatAnswers formats direct answers as a readable string.
func formatAnswers(answers []Answer) string {
	if len(answers) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("=== Answers ===\n\n")
	for i, a := range answers {
		b.WriteString(fmt.Sprintf("[%d] %s\n", i+1, a.Answer))
		if a.Engine != "" {
			b.WriteString(fmt.Sprintf("    Engine: %s\n", a.Engine))
		}
	}
	b.WriteString("\n")
	return b.String()
}

// formatInfoboxes formats infoboxes as a readable string.
func formatInfoboxes(infoboxes []Infobox) string {
	if len(infoboxes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("=== Infoboxes ===\n\n")
	for i, ib := range infoboxes {
		b.WriteString(fmt.Sprintf("[%d] %s\n", i+1, ib.Infobox))
		if ib.Content != "" {
			content := unescapeIfNeeded(ib.Content)
			if utf8.RuneCountInString(content) > MaxContentRunes {
				runes := []rune(content)
				content = string(runes[:MaxContentRunes])
			}
			b.WriteString(fmt.Sprintf("    %s\n", content))
		}
		if len(ib.Attributes) > 0 {
			b.WriteString("    Attributes:\n")
			for _, attr := range ib.Attributes {
				b.WriteString(fmt.Sprintf("      - %s: %s\n", attr.Label, attr.Value))
			}
		}
		if len(ib.URLs) > 0 {
			b.WriteString("    URLs:\n")
			for _, u := range ib.URLs {
				b.WriteString(fmt.Sprintf("      - %s: %s\n", u.Title, u.URL))
			}
		}
		if i < len(infoboxes)-1 {
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}

// formatResults formats search results as a readable string
func formatResults(resp *SearchResponse) string {
	if resp == nil {
		return "No results found."
	}
	if len(resp.Results) == 0 && len(resp.Infoboxes) == 0 && len(resp.Answers) == 0 {
		return "No results found."
	}

	var b strings.Builder

	// Answers first (direct answers like IP, hash, timezone)
	if ansText := formatAnswers(resp.Answers); ansText != "" {
		b.WriteString(ansText)
	}

	// Infoboxes
	if ibText := formatInfoboxes(resp.Infoboxes); ibText != "" {
		b.WriteString(ibText)
	}

	// Results
	if len(resp.Results) > 0 {
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
	}

	// Suggestions last
	if len(resp.Suggestions) > 0 {
		b.WriteString("Suggestions:\n")
		for _, s := range resp.Suggestions {
			b.WriteString(fmt.Sprintf("  - %s\n", s))
		}
	}
	return b.String()
}
