package main

import (
	"html"
	"log/slog"
	"strconv"
	"strings"
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

// truncateRunes truncates s to at most limit runes in a single pass.
// It returns the original string unchanged if already within the limit.
func truncateRunes(s string, limit int) string {
	if limit <= 0 || s == "" {
		return ""
	}
	runeCount := 0
	for i := range s {
		if runeCount == limit {
			return s[:i]
		}
		runeCount++
	}
	return s
}

// writeAnswers writes formatted direct answers to b.
func writeAnswers(b *strings.Builder, answers []Answer) {
	if len(answers) == 0 {
		return
	}
	b.WriteString("=== Answers ===\n\n")
	for i, a := range answers {
		b.WriteByte('[')
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString("] ")
		b.WriteString(a.Answer)
		b.WriteByte('\n')
		if a.Engine != "" {
			b.WriteString("    Engine: ")
			b.WriteString(a.Engine)
			b.WriteByte('\n')
		}
	}
	b.WriteByte('\n')
}

// writeInfoboxes writes formatted infoboxes to b.
func writeInfoboxes(b *strings.Builder, infoboxes []Infobox) {
	if len(infoboxes) == 0 {
		return
	}
	b.WriteString("=== Infoboxes ===\n\n")
	for i, ib := range infoboxes {
		b.WriteByte('[')
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString("] ")
		b.WriteString(ib.Infobox)
		b.WriteByte('\n')
		if ib.Content != "" {
			content := unescapeIfNeeded(ib.Content)
			content = truncateRunes(content, MaxContentRunes)
			b.WriteString("    ")
			b.WriteString(content)
			b.WriteByte('\n')
		}
		if len(ib.Attributes) > 0 {
			b.WriteString("    Attributes:\n")
			for _, attr := range ib.Attributes {
				b.WriteString("      - ")
				b.WriteString(attr.Label)
				b.WriteString(": ")
				b.WriteString(attr.Value)
				b.WriteByte('\n')
			}
		}
		if len(ib.URLs) > 0 {
			b.WriteString("    URLs:\n")
			for _, u := range ib.URLs {
				b.WriteString("      - ")
				b.WriteString(u.Title)
				b.WriteString(": ")
				b.WriteString(u.URL)
				b.WriteByte('\n')
			}
		}
		if i < len(infoboxes)-1 {
			b.WriteByte('\n')
		}
	}
	b.WriteByte('\n')
}

func logUnresponsiveEngines(resp *SearchResponse) {
	if resp == nil || !resp.Debug || len(resp.UnresponsiveEngines) == 0 {
		return
	}
	for _, entry := range resp.UnresponsiveEngines {
		if len(entry) < 2 {
			slog.Debug("unresponsive engine", "entry", entry)
			continue
		}
		slog.Debug("unresponsive engine", "engine", entry[0], "error", entry[1])
	}
}

// formatResults formats search results as a readable string
func formatResults(resp *SearchResponse) string {
	logUnresponsiveEngines(resp)

	if resp == nil {
		return "No results found."
	}
	if len(resp.Results) == 0 && len(resp.Infoboxes) == 0 && len(resp.Answers) == 0 && len(resp.Suggestions) == 0 {
		return "No results found."
	}

	var b strings.Builder

	// Answers first (direct answers like IP, hash, timezone)
	writeAnswers(&b, resp.Answers)

	// Infoboxes
	writeInfoboxes(&b, resp.Infoboxes)

	// Results
	if len(resp.Results) > 0 {
		b.WriteString("=== Results ===\n\n")
		total := resp.NumberOfResults
		if total == 0 {
			total = len(resp.Results)
		}
		b.WriteString("Found ")
		b.WriteString(strconv.Itoa(total))
		b.WriteString(" results for '")
		b.WriteString(resp.Query)
		b.WriteString("':\n\n")
		for i, r := range resp.Results {
			title := unescapeIfNeeded(r.Title)
			b.WriteString(strconv.Itoa(i + 1))
			b.WriteString(". ")
			b.WriteString(title)
			b.WriteByte('\n')
			b.WriteString("   URL: ")
			b.WriteString(r.URL)
			b.WriteByte('\n')
			if r.Content != "" {
				content := unescapeIfNeeded(r.Content)
				content = truncateRunes(content, MaxContentRunes)
				b.WriteString("   Summary: ")
				b.WriteString(content)
				b.WriteByte('\n')
			}
			if r.PublishedDate != nil && *r.PublishedDate != "" {
				b.WriteString("   Published date: ")
				b.WriteString(*r.PublishedDate)
				b.WriteByte('\n')
			}
			b.WriteString("   Engine: ")
			b.WriteString(r.Engine)
			b.WriteString("\n\n")
		}
	}

	// Suggestions last
	if len(resp.Suggestions) > 0 {
		b.WriteString("=== Search Suggestions ===\n\n")
		for _, s := range resp.Suggestions {
			b.WriteString("  - ")
			b.WriteString(s)
			b.WriteByte('\n')
		}
	}
	return b.String()
}
